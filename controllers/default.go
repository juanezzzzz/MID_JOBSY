package controllers

import (
	beego "github.com/beego/beego/v2/server/web"
)

type MainController struct {
	beego.Controller
}

func (c *MainController) Health() {
	c.Data["json"] = map[string]interface{}{
		"status":  "ok",
		"service": "MID_JOBSY",
		"message": "API MID funcionando correctamente",
	}
	c.ServeJSON()
}
