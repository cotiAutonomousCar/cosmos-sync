//init client from clientPool.
//client is httpClient of tendermint

package pool

import (
	"context"
	"errors"
	"github.com/bianjieai/cosmos-sync/config"
	"github.com/bianjieai/cosmos-sync/libs/logger"
	"github.com/bianjieai/cosmos-sync/resource"
	commonPool "github.com/jolestar/go-commons-pool"
	"strings"
	"sync"
	"time"
)

var (
	poolObject  *commonPool.ObjectPool
	poolFactory PoolFactory
	ctx         = context.Background()
)

func Init(conf *config.Config) {
	var (
		syncMap sync.Map
	)
	nodeRpcs, err := resource.GetRpcNodesFromGithubRepo(conf.Server.ChainId)
	if err != nil {
		//从github获取失败退出
		logger.Fatal("GetRpcNodesFromGithubRepo fail " + err.Error())
		return
	}
	if len(nodeRpcs) == 0 {
		logger.Fatal("no found Rpc Nodes From GithubRepo")
	}
	nodeUrls := strings.Split(nodeRpcs, ",")
	for _, url := range nodeUrls {
		key := generateId(url)
		endPoint := EndPoint{
			Address:   url,
			Available: true,
		}

		syncMap.Store(key, endPoint)
	}

	poolFactory = PoolFactory{
		chainId:  conf.Server.ChainId,
		peersMap: syncMap,
	}

	config := commonPool.NewDefaultPoolConfig()
	config.MaxTotal = conf.Server.MaxConnectionNum
	config.MaxIdle = conf.Server.InitConnectionNum
	config.MinIdle = conf.Server.InitConnectionNum
	config.TestOnBorrow = true
	config.TestOnCreate = true
	config.TestWhileIdle = true

	poolObject = commonPool.NewObjectPool(ctx, &poolFactory, config)
	poolObject.PreparePool(ctx)
}

// get client from pool
func GetClient() *Client {
	c, err := poolObject.BorrowObject(ctx)
	for err != nil {
		logger.Error("GetClient failed,will try again after 3 seconds", logger.String("err", err.Error()))
		time.Sleep(3 * time.Second)
		c, err = poolObject.BorrowObject(ctx)
	}

	return c.(*Client)
}

// release client
func (c *Client) Release() {
	err := poolObject.ReturnObject(ctx, c)
	if err != nil {
		logger.Error(err.Error())
	}
}

func (c *Client) HeartBeat() error {
	http := c.HTTP
	_, err := http.Health(context.Background())
	return err
}

func (c *Client) InvalidateObject() {
	err := poolObject.InvalidateObject(ctx, c)
	if err != nil {
		logger.Error(err.Error())
	}
}

func ClosePool() {
	poolObject.Close(ctx)
}

func GetClientWithTimeout(timeout time.Duration) (*Client, error) {
	c := make(chan interface{})
	errCh := make(chan error)
	go func() {
		client, err := poolObject.BorrowObject(ctx)
		if err != nil {
			errCh <- err
		} else {
			c <- client
		}
	}()
	select {
	case res := <-c:
		return res.(*Client), nil
	case res := <-errCh:
		return nil, res
	case <-time.After(timeout):
		return nil, errors.New("rpc node timeout")
	}
}
