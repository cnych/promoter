package notify

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	prometheus "github.com/prometheus/client_golang/api"
	prometheusApi "github.com/prometheus/client_golang/api/prometheus/v1"
	promModel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/palette/brewer"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

type AlertImage struct {
	Url   string `json:"url"`
	Title string `json:"title"`
}

type PlotExpr struct {
	Formula  string
	Operator string
	Level    float64
}

func (expr PlotExpr) String() string {
	return fmt.Sprintf("%s %s %.2f", expr.Formula, expr.Operator, expr.Level)
}

// Only show important part of metric name
var labelText = regexp.MustCompile("{(.*)}")

func GetPlotExpr(logger log.Logger, alertFormula string) []PlotExpr {
	expr, _ := promql.ParseExpr(alertFormula)
	if parenExpr, ok := expr.(*promql.ParenExpr); ok {
		expr = parenExpr.Expr
		level.Debug(logger).Log("msg", "Removing redundant brackets", "expr", expr.String())
	}

	if binaryExpr, ok := expr.(*promql.BinaryExpr); ok {
		var alertOperator string

		switch binaryExpr.Op {
		case promql.ItemLAND:
			level.Debug(logger).Log("msg", "Logical condition, drawing sides separately")
			return append(GetPlotExpr(logger, binaryExpr.LHS.String()), GetPlotExpr(logger, binaryExpr.RHS.String())...)
		case promql.ItemLTE, promql.ItemLSS:
			alertOperator = "<"
		case promql.ItemGTE, promql.ItemGTR:
			alertOperator = ">"
		default:
			level.Debug(logger).Log("msg", "Unexpected operator", "Op", binaryExpr.Op.String())
			alertOperator = ">"
		}

		alertLevel, _ := strconv.ParseFloat(binaryExpr.RHS.String(), 64)
		return []PlotExpr{{
			Formula:  binaryExpr.LHS.String(),
			Operator: alertOperator,
			Level:    alertLevel,
		}}
	} else {
		level.Debug(logger).Log("msg", "Non binary expression", "expr", alertFormula)
		return nil
	}
}

func Plot(logger log.Logger, expr PlotExpr, queryTime time.Time, duration, resolution time.Duration, prometheusUrl string, alert Alert) (io.WriterTo, error) {
	level.Debug(logger).Log("msg", "Querying Prometheus", "expr", expr.Formula)
	metrics, err := Metrics(
		prometheusUrl,
		expr.Formula,
		queryTime,
		duration,
		resolution,
	)
	if err != nil {
		return nil, err
	}

	var selectedMetrics promModel.Matrix
	var founded bool
	for _, metric := range metrics {
		founded = false
		for label, value := range metric.Metric {
			if originValue, ok := alert.Labels[string(label)]; ok {
				if originValue == string(value) {
					founded = true
				} else {
					founded = false
					break
				}
			}
		}

		if founded {
			level.Debug(logger).Log("msg", "Best match founded", "metric", metric.Metric)
			selectedMetrics = promModel.Matrix{metric}
			break
		}
	}

	if !founded {
		level.Debug(logger).Log("msg", "Best match not founded, use entire dataset. Labels to search", "labels", alert.Labels.Values())
		selectedMetrics = metrics
	}

	level.Debug(logger).Log("msg", "Creating plot", "summary", alert.Annotations["summary"])

	plottedMetric, err := PlotMetric(selectedMetrics, expr.Level, expr.Operator)
	if err != nil {
		return nil, err
	}

	return plottedMetric, nil
}

func PlotMetric(metrics promModel.Matrix, level float64, direction string) (io.WriterTo, error) {
	p, err := plot.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create new plot: %v", err)
	}

	textFont, err := vg.MakeFont("Helvetica", 3*vg.Millimeter)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %v", err)
	}

	evalTextFont, err := vg.MakeFont("Helvetica", 5*vg.Millimeter)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %v", err)
	}

	evalTextStyle := draw.TextStyle{
		Color:  color.NRGBA{A: 150},
		Font:   evalTextFont,
		XAlign: draw.XRight,
		YAlign: draw.YBottom,
	}

	p.X.Tick.Marker = plot.TimeTicks{Format: "15:04:05"}
	p.X.Tick.Label.Font = textFont
	p.Y.Tick.Label.Font = textFont
	p.Legend.Font = textFont
	p.Legend.Top = true
	p.Legend.YOffs = 15 * vg.Millimeter

	// Color palette for drawing lines
	paletteSize := 8
	palette, err := brewer.GetPalette(brewer.TypeAny, "Dark2", paletteSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get color palette: %v", err)
	}
	colors := palette.Colors()

	var lastEvalValue float64

	for s, sample := range metrics {
		data := make(plotter.XYs, 0)
		for _, v := range sample.Values {
			fs := v.Value.String()
			if fs == "NaN" {
				_, err := drawLine(data, colors, s, paletteSize, p, metrics, sample)
				if err != nil {
					return nil, err
				}

				data = make(plotter.XYs, 0)
				continue
			}

			f, err := strconv.ParseFloat(fs, 64)
			if err != nil {
				return nil, fmt.Errorf("sample value not float: %s", v.Value.String())
			}
			data = append(data, plotter.XY{X: float64(v.Timestamp.Unix()), Y: f})
			lastEvalValue = f
		}

		_, err := drawLine(data, colors, s, paletteSize, p, metrics, sample)
		if err != nil {
			return nil, err
		}
	}

	var polygonPoints plotter.XYs

	if direction == "<" {
		polygonPoints = plotter.XYs{{X: p.X.Min, Y: level}, {X: p.X.Max, Y: level}, {X: p.X.Max, Y: p.Y.Min}, {X: p.X.Min, Y: p.Y.Min}}
	} else {
		polygonPoints = plotter.XYs{{X: p.X.Min, Y: level}, {X: p.X.Max, Y: level}, {X: p.X.Max, Y: p.Y.Max}, {X: p.X.Min, Y: p.Y.Max}}
	}

	poly, err := plotter.NewPolygon(polygonPoints)
	if err != nil {
		return nil, err
	}
	poly.Color = color.NRGBA{R: 255, A: 40}
	poly.LineStyle.Color = color.NRGBA{R: 0, A: 0}
	p.Add(poly)
	p.Add(plotter.NewGrid())

	// Draw plot in canvas with margin
	margin := 6 * vg.Millimeter
	width := 20 * vg.Centimeter
	height := 10 * vg.Centimeter
	c, err := draw.NewFormattedCanvas(width, height, "png")
	if err != nil {
		return nil, fmt.Errorf("failed to create canvas: %v", err)
	}

	cropedCanvas := draw.Crop(draw.New(c), margin, -margin, margin, -margin)
	p.Draw(cropedCanvas)

	// Draw last evaluated value
	evalText := fmt.Sprintf("latest evaluation: %.2f", lastEvalValue)

	plotterCanvas := p.DataCanvas(cropedCanvas)

	trX, trY := p.Transforms(&plotterCanvas)
	evalRectangle := evalTextStyle.Rectangle(evalText)

	points := []vg.Point{
		{X: trX(p.X.Max) + evalRectangle.Min.X - 8*vg.Millimeter, Y: trY(lastEvalValue) + evalRectangle.Min.Y - vg.Millimeter},
		{X: trX(p.X.Max) + evalRectangle.Min.X - 8*vg.Millimeter, Y: trY(lastEvalValue) + evalRectangle.Max.Y + vg.Millimeter},
		{X: trX(p.X.Max) + evalRectangle.Max.X - 6*vg.Millimeter, Y: trY(lastEvalValue) + evalRectangle.Max.Y + vg.Millimeter},
		{X: trX(p.X.Max) + evalRectangle.Max.X - 6*vg.Millimeter, Y: trY(lastEvalValue) + evalRectangle.Min.Y - vg.Millimeter},
	}
	plotterCanvas.FillPolygon(color.NRGBA{R: 255, G: 255, B: 255, A: 90}, points)
	plotterCanvas.FillText(evalTextStyle, vg.Point{X: trX(p.X.Max) - 6*vg.Millimeter, Y: trY(lastEvalValue)}, evalText)

	return c, nil
}

func drawLine(data plotter.XYs, colors []color.Color, s int, paletteSize int, p *plot.Plot, metrics promModel.Matrix, sample *promModel.SampleStream) (*plotter.Line, error) {
	var l *plotter.Line
	var err error
	if len(data) > 0 {
		l, err = plotter.NewLine(data)
		if err != nil {
			return &plotter.Line{}, fmt.Errorf("failed to create line: %v", err)
		}

		l.LineStyle.Width = vg.Points(1)
		l.LineStyle.Color = colors[s%paletteSize]

		p.Add(l)
		if len(metrics) > 1 {
			m := labelText.FindStringSubmatch(sample.Metric.String())
			if m != nil {
				p.Legend.Add(m[1], l)
			}
		}
	}

	return l, nil
}

func Metrics(server, query string, queryTime time.Time, duration, step time.Duration) (promModel.Matrix, error) {
	client, err := prometheus.NewClient(prometheus.Config{Address: server})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %v", err)
	}

	api := prometheusApi.NewAPI(client)
	value, _, err := api.QueryRange(context.Background(), query, prometheusApi.Range{
		Start: queryTime.Add(-duration),
		End:   queryTime,
		Step:  duration / step,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query Prometheus: %v", err)
	}

	metrics, ok := value.(promModel.Matrix)
	if !ok {
		return nil, fmt.Errorf("unsupported result format: %s", value.Type().String())
	}

	return metrics, nil
}
