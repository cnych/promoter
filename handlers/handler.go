package handlers

import (
	"fmt"
	"log"
	"net/http/httputil"

	"github.com/cnych/promoter/models"
	"github.com/cnych/promoter/notifier"
	"github.com/cnych/promoter/template"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func Healthz(c *gin.Context) {
	c.JSON(200, gin.H{
		"ping": "ok",
	})
}

func WebhookHandler(tmpl *template.Template, conf *models.Config) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		if viper.GetBool("debug") {
			// 为 debug dump 一份请求
			requestDump, err := httputil.DumpRequest(c.Request, true)
			if err != nil {
				fmt.Println(err)
			} else {
				log.Printf("New request")
				fmt.Println(string(requestDump))
			}
		}

		var hookMessage models.Data
		if err := c.ShouldBindJSON(&hookMessage); err != nil {
			c.JSON(400, map[string]string{"success": "false", "message": err.Error()})
		} else {
			builder := notifier.NewDingNotificationBuilder(tmpl, conf)
			resp, err := builder.SendNotification(&hookMessage)
			if err != nil {
				c.JSON(400, map[string]string{"success": "false", "message": err.Error()})
				return
			}

			if resp.ErrorCode != 0 {
				c.JSON(400, map[string]string{"success": "false", "message": resp.ErrorMessage})
				return
			}

			c.JSON(200, map[string]string{"success": "true"})

		}
	}
	return gin.HandlerFunc(fn)
}

