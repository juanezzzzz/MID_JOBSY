package main

import (
	_ "api_MID/routers"
	beego "github.com/beego/beego/v2/server/web"
)

func main() {
	beego.Run()
}

