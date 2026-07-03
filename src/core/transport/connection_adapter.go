package transport

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core"
	"xiaozhi-server-go/src/core/provider"
	"xiaozhi-server-go/src/core/utils"
)

type ConnectionContextAdapter struct {
	handler     *core.ConnectionHandler
	providerSet *provider.ProviderSet
	providerMgr *provider.ProviderManager
	clientID    string
	logger      *utils.Logger
	conn        Connection
	ctx         context.Context
	cancel      context.CancelFunc
	closed      int32
}

func NewConnectionContextAdapter(
	conn Connection,
	config *configs.Config,
	providerSet *provider.ProviderSet,
	providerMgr *provider.ProviderManager,
	logger *utils.Logger,
	req *http.Request,
) *ConnectionContextAdapter {
	clientID := conn.GetID()
	connCtx, connCancel := context.WithCancel(context.Background())

	handler := core.NewConnectionHandler(config, providerSet, logger, req, connCtx)

	adapter := &ConnectionContextAdapter{
		handler:     handler,
		providerSet: providerSet,
		providerMgr: providerMgr,
		clientID:    clientID,
		logger:      logger,
		conn:        conn,
		ctx:         connCtx,
		cancel:      connCancel,
	}

	return adapter
}

func (a *ConnectionContextAdapter) Handle() {
	a.handler.Handle(a.conn)
	a.logger.Info("client %s connection handling complete", a.clientID)
}

func (a *ConnectionContextAdapter) Close() {
	if !atomic.CompareAndSwapInt32(&a.closed, 0, 1) {
		a.logger.Info("client %s connection already closed", a.clientID)
		return
	}

	a.cancel()

	if a.handler != nil {
		a.handler.Close()
	}
	if a.conn != nil {
		a.conn.Close()
	}
	if a.providerSet != nil && a.providerMgr != nil {
		if err := a.providerMgr.ReturnProviderSet(a.providerSet); err != nil {
			a.logger.Error("client %s provider cleanup failed: %v", a.clientID, err)
		} else {
			a.logger.Info("client %s providers cleaned up", a.clientID)
		}
	}
}

func (a *ConnectionContextAdapter) GetSessionID() string {
	return a.clientID
}

func (a *ConnectionContextAdapter) IsActive() bool {
	return atomic.LoadInt32(&a.closed) == 0
}

func (a *ConnectionContextAdapter) GetContext() context.Context {
	return a.ctx
}

func (a *ConnectionContextAdapter) GetConnectionHandler() *core.ConnectionHandler {
	return a.handler
}

type DefaultConnectionHandlerFactory struct {
	config      *configs.Config
	providerMgr *provider.ProviderManager
	logger      *utils.Logger
}

func NewDefaultConnectionHandlerFactory(
	config *configs.Config,
	providerMgr *provider.ProviderManager,
	logger *utils.Logger,
) *DefaultConnectionHandlerFactory {
	return &DefaultConnectionHandlerFactory{
		config:      config,
		providerMgr: providerMgr,
		logger:      logger,
	}
}

func (f *DefaultConnectionHandlerFactory) CreateHandler(
	conn Connection,
	req *http.Request,
) ConnectionHandler {
	providerSet, err := f.providerMgr.GetProviderSet()
	if err != nil {
		f.logger.Error(fmt.Sprintf("create provider set failed: %v", err))
		return nil
	}

	if holder, ok := conn.(MCPManagerHolder); ok {
		if mgr := holder.GetMCPManager(); mgr != nil {
			if providerSet.MCP != nil {
				_ = providerSet.MCP.Cleanup()
			}
			providerSet.MCP = mgr
		}
	}

	return NewConnectionContextAdapter(
		conn,
		f.config,
		providerSet,
		f.providerMgr,
		f.logger,
		req,
	)
}
