package main

import (
	"fmt"

	"github.com/cnych/promoter/handlers"
	"github.com/cnych/promoter/models"
	"github.com/cnych/promoter/template"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)



func main() {
	// 设置配置文件
	//viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/promoter")
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")

	// 查找并读取配置文件
	if err:= viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("Error read config file: %s \n", err))
	}
	// 检查环境变量
	viper.AutomaticEnv()
	// 设置环境变量前缀
	viper.SetEnvPrefix("promoter")

	// 读取配置文件
	conf := models.Config{}
	if err := viper.Unmarshal(&conf); err != nil {
		panic(err)
	}
	// 渲染配置的模板文件
	tmpl, err := template.FromGlobs(true, conf.Templates...)
	if err != nil {
		panic(err)
	}

	r := gin.New()
	r.Use(gin.LoggerWithWriter(gin.DefaultWriter, "/healthz"))
	r.Use(gin.Recovery())

	r.GET("/healthz", handlers.Healthz)
	r.POST("/webhook", handlers.WebhookHandler(tmpl, &conf))

	if err := r.Run(":" + viper.GetString("http_port")); err != nil {
		panic(fmt.Errorf("Error start webhook server: %s \n", err))
	}
}