package main

import (
	"context"
	"fmt"
	"time"

	safetyreminder "wechat-robot-client/pkg/safetyreminder"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	w, err := safetyreminder.FetchWeather(ctx, "101230501")
	if err != nil {
		fmt.Println("ERR:", err)
		return
	}
	fmt.Println("天气:", w.Summary())
	fmt.Println("判定:", w.Kind())
}
