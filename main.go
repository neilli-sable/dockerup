package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	_ "github.com/go-sql-driver/mysql" // blank import for dbr
	"github.com/gocraft/dbr"
	"github.com/moby/moby/client"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	// コンテナの立ち上げ
	config := &container.Config{
		Image:        "mysql",
		ExposedPorts: nat.PortSet{nat.Port("3306"): struct{}{}},
		Cmd:          []string{"--default-authentication-plugin=mysql_native_password"},
		Env: []string{
			"MYSQL_ROOT_PASSWORD=root",
			"MYSQL_PASSWORD=root",
			"MYSQL_DATABASE=dockerup",
		},
	}
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port("3306"): []nat.PortBinding{{HostPort: "7706"}},
		},
		AutoRemove: true,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: path.Join(dir, "schema"),
				Target: "/docker-entrypoint-initdb.d",
			},
		},
	}
	netConfig := &network.NetworkingConfig{}

	log.Println("Container creating...")
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, netConfig, "dockerup")
	if err != nil {
		panic(err)
	}
	log.Printf("Created! ContainerID is %v", resp.ID)

	log.Println("Container starting...")
	err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		panic(err)
	}
	log.Println("Container Started!")

	log.Println("Starting mysql process")

	sess, err := GetMySQLSession()
	if err != nil {
		panic(err)
	}
	{
		log.Println("Wait...")
		count := 0
		maxRetry := 10
		for {
			time.Sleep(5 * time.Second)
			if err := sess.Ping(); err != nil {
				count++
				if count < maxRetry {
					continue
				}
				log.Println("Over max retry count")
				log.Println("Bye")
				panic(err)
			}
			break
		}
	}
	log.Println("OK! Container started!")

	type Result struct {
		ID int `db:"id"`
	}
	result := &Result{}
	err = sess.Select("*").From("dockerup").LoadOne(result)
	if err != nil {
		panic(err)
	}
	log.Printf("DB access is Succeed! Stored ID is %d", result.ID)

	// コンテナを破棄する
	log.Println("Container stopping...")
	timeout := time.Duration(60 * time.Second)
	err = cli.ContainerStop(ctx, resp.ID, &timeout)
	if err != nil {
		panic(err)
	}
	log.Println("Container stopped!")
	log.Println("Maybe, container is already removed by \"AutoRemove\" option")
	log.Println("ALL DONE!!!")
}

// GetMySQLSession MySQLセッションの作成
func GetMySQLSession() (*dbr.Session, error) {
	info := DBEnv{
		Host:     "localhost",
		Port:     7706,
		User:     "root",
		Password: "root",
		DBName:   "dockerup",
		Protocol: "tcp",
		Param:    "?parseTime=true",
	}
	con, err := dbr.Open("mysql",
		info.User+":"+info.Password+"@"+info.Protocol+"("+info.Host+":"+fmt.Sprint(info.Port)+")/"+info.DBName+info.Param, nil)
	if err != nil {
		return nil, err
	}
	sess := con.NewSession(nil)
	return sess, nil
}

// DBEnv ...
type DBEnv struct {
	Host     string
	Port     uint64
	User     string
	Password string
	DBName   string
	Protocol string
	Param    string
}
