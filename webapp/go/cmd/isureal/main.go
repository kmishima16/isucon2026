package main

import (
	"context"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"example.com/isureal-company/isureal"
)

// 2026-09-23

func main() {
	// 初期データの投稿の画像をこの台のディスクに書き出す（initial_image_store.go）。ベンチの外で一度だけ流す
	if len(os.Args) > 1 && os.Args[1] == "extract-initial-images" {
		if err := isureal.ExtractInitialImages(context.Background(), isureal.ConfigFromEnv()); err != nil {
			log.Fatal(err)
		}
		return
	}
	// DB の外の CPU を pprotein で見るため。最終盤は ISU_MEASURE=0（make measure-off）で待ち受けない。外からは届かない localhost だけ
	if os.Getenv("ISU_MEASURE") != "0" {
		go func() {
			_ = http.ListenAndServe("localhost:6060", nil)
		}()
	}
	// 署名の窓口（image_signer.go）。VPC の中のほかの台からだけ届く
	go func() {
		if err := http.ListenAndServe(":8081", isureal.NewSignerHandler()); err != nil {
			log.Printf("signer listener stopped: %v", err)
		}
	}()
	config := isureal.ConfigFromEnv()
	server := isureal.NewServer(config)
	if err := server.Start(":8080"); err != nil {
		panic("failed to start server: " + err.Error())
	}
}
