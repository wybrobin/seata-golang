package main

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"

	"github.com/opentrx/seata-golang/v2/pkg/apis"
	"github.com/opentrx/seata-golang/v2/pkg/tc/config"
	_ "github.com/opentrx/seata-golang/v2/pkg/tc/metrics"
	"github.com/opentrx/seata-golang/v2/pkg/tc/server"
	_ "github.com/opentrx/seata-golang/v2/pkg/tc/storage/driver/inmemory"
	_ "github.com/opentrx/seata-golang/v2/pkg/tc/storage/driver/mysql"
	_ "github.com/opentrx/seata-golang/v2/pkg/tc/storage/driver/pgsql"
	"github.com/opentrx/seata-golang/v2/pkg/util/log"
	"github.com/opentrx/seata-golang/v2/pkg/util/uuid"
)

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "start seata golang tc server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Load configuration from `FILE`",
					},
					&cli.StringFlag{
						Name:    "serverNode",
						Aliases: []string{"n"},
						Value:   "1",
						Usage:   "server node id, such as 1, 2, 3. default is 1",
					},
				},
				Action: func(c *cli.Context) error {
					configPath := c.String("config")
					serverNode := c.Int64("serverNode")

					//读取配置文件到结构体 config.Configuration 里
					cfg, err := resolveConfiguration(configPath)
					if err != nil || cfg == nil {
						return err
					}

					_ = uuid.Init(serverNode)
					log.Init(cfg.Log.LogPath, cfg.Log.LogLevel)

					address := fmt.Sprintf(":%v", cfg.Server.Port)
					lis, err := net.Listen("tcp", address)
					if err != nil {
						log.Fatalf("failed to listen: %v", err)
					}

					//grpc keepalive配置
					s := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(cfg.GetEnforcementPolicy()),
						grpc.KeepaliveParams(cfg.GetServerParameters()))

					//生成tc对象，并且启动4个gorouting
					tc := server.NewTransactionCoordinator(cfg)
					//注册grpc的TransactionManagerService，给tm提供接口
					apis.RegisterTransactionManagerServiceServer(s, tc)
					//注册grpc的ResourceManagerService，给tc提供接口
					apis.RegisterResourceManagerServiceServer(s, tc)

					go func() {
						//启动了一个http端口为10001的服务，不知道做什么？？？
						err = http.ListenAndServe(":10001", nil)
						if err != nil {
							return
						}
					}()

					if err := s.Serve(lis); err != nil {
						log.Fatalf("failed to serve: %v", err)
					}
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
	}
}

func resolveConfiguration(configPath string) (*config.Configuration, error) {
	var configurationPath string

	if configPath != "" {
		configurationPath = configPath
	} else if os.Getenv("SEATA_CONFIGURATION_PATH") != "" {
		configurationPath = os.Getenv("SEATA_CONFIGURATION_PATH")
	}

	if configurationPath == "" {
		return nil, fmt.Errorf("configuration path unspecified")
	}

	fp, err := os.Open(configurationPath)
	if err != nil {
		return nil, err
	}

	defer func(fp *os.File) {
		err = fp.Close()
		if err != nil {
			log.Error(err)
		}
	}(fp)

	cfg, err := config.Parse(fp)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", configurationPath, err)
	}

	return cfg, nil
}
