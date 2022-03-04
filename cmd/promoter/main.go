package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/cnych/promoter/api"
	"github.com/cnych/promoter/config"
	"github.com/cnych/promoter/template"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	promlogConfig = promlog.Config{}
	term          = make(chan os.Signal, 1)
)

func main() {
	os.Exit(run())
}

func run() int {
	if os.Getenv("DEBUG") != "" {
		runtime.SetBlockProfileRate(20)
		runtime.SetMutexProfileFraction(20)
	}

	var (
		configFile    = kingpin.Flag("config.file", "Promoter configuration file.").Default("config.yaml").ExistingFile()
		debug         = kingpin.Flag("web.debug", "Dump request data").Default("false").Bool()
		externalURL   = kingpin.Flag("web.external-url", "The URL under which Promoter is externally reachable (for example, if Promoter is served via a reverse proxy). Used for generating relative and absolute links back to Promoter itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Promoter. If omitted, relevant URL components will be derived automatically.").String()
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for the web interface and API.").Default(":8080").String()
	)

	promlogflag.AddFlags(kingpin.CommandLine, &promlogConfig)

	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.Version(version.Print("promoter"))
	kingpin.CommandLine.GetFlag("help").Short('h')
	kingpin.Parse()

	logger := promlog.New(&promlogConfig)

	level.Info(logger).Log("msg", "Staring Promoter", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	// 加载配置文件和模板
	conf, err := loadConfiguration(logger, *configFile)
	if err != nil {
		return 1
	}
	amURL, err := extURL(logger, os.Hostname, *listenAddress, *externalURL)
	if err != nil {
		level.Error(logger).Log("msg", "failed to determine external URL", "err", err)
		return 1
	}
	level.Debug(logger).Log("msg", "parse promoter external url", "url", amURL.String())
	tmpl, err := loadTemplate(logger, conf, amURL)
	if err != nil {
		return 1
	}

	api := api.New(api.Options{
		Logger: logger,
		Debug:  *debug,
	})
	api.Update(conf, tmpl) // 更新配置对象

	mux := api.Register(route.New()) // 注册路由
	srv := http.Server{Addr: *listenAddress, Handler: mux}
	srvc := make(chan struct{})

	go func() {
		level.Info(logger).Log("msg", "Listening", "address", *listenAddress)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			level.Error(logger).Log("msg", "Listen error", "err", err)
			close(srvc)
		}
		defer func() {
			if err := srv.Close(); err != nil {
				level.Error(logger).Log("msg", "Error on closing the server", "err", err)
			}
		}()
	}()

	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-term:
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
			return 0
		case <-srvc:
			return 1
		}
	}
}

func loadTemplate(logger log.Logger, conf *config.Config, externalURL *url.URL) (*template.Template, error) {
	tmplLogger := log.With(logger, "component", "template")

	tmpl, err := template.FromGlobs(conf.Templates...)
	if err != nil {
		level.Error(tmplLogger).Log("msg", errors.Wrap(err, "failed to parse templates"))
		return nil, err
	}
	tmpl.ExternalURL = externalURL
	return tmpl, nil
}

func loadConfiguration(logger log.Logger, configFilePath string) (*config.Config, error) {
	configLogger := log.With(logger, "component", "configuration")
	level.Info(configLogger).Log("msg", "Loading configuration file", "file", configFilePath)

	// 加载配置文件和模板
	conf, err := config.LoadFile(configFilePath)
	if err != nil {
		level.Error(configLogger).Log(
			"msg", "Loading configuration file failed",
			"file", configFilePath,
			"err", err)
		return nil, err
	}
	level.Info(configLogger).Log("msg", "Completed loading of configuration file", "file", configFilePath)
	return conf, nil
}

func extURL(logger log.Logger, hostnamef func() (string, error), listen, external string) (*url.URL, error) {
	if external == "" {
		hostname, err := hostnamef()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(listen)
		if err != nil {
			return nil, err
		}
		if port == "" {
			level.Warn(logger).Log("msg", "no port found for listen address", "address", listen)
		}

		external = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	u, err := url.Parse(external)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.Errorf("%q: invalid %q scheme, only 'http' and 'https' are supported", u.String(), u.Scheme)
	}

	ppref := strings.TrimRight(u.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	u.Path = ppref

	return u, nil
}
