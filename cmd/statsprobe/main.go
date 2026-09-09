package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v2rayapi "github.com/the-lpn-foundation/sing-box-agent/internal/v2rayapi"
)

func main() {
	addr := os.Args[1]
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(v2rayapi.Codec{})),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = conn.Close() }()
	req := &v2rayapi.QueryStatsRequest{Patterns: []string{">>>"}}
	resp := &v2rayapi.QueryStatsResponse{}
	err = conn.Invoke(context.Background(), "/v2ray.core.app.stats.command.StatsService/QueryStats", req, resp, grpc.StaticMethod())
	if err != nil {
		panic(err)
	}
	fmt.Println("stats len:", len(resp.GetStat()))
	for _, s := range resp.GetStat() {
		fmt.Println(s.GetName(), "=", s.GetValue())
	}
	// sysstats to confirm service works
	sysReq := &v2rayapi.SysStatsRequest{}
	sysResp := &v2rayapi.SysStatsResponse{}
	err = conn.Invoke(context.Background(), "/v2ray.core.app.stats.command.StatsService/GetSysStats", sysReq, sysResp)
	fmt.Println("second call err:", err)
}
