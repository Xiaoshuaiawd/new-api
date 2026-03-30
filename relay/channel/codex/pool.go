package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gorilla/websocket"
)

const maxResponsesWebSocketConnectionsPerChannel = 15

var errResponsesWebSocketPoolClosed = errors.New("codex websocket pool closed")

var (
	responsesWebSocketPoolsMu sync.Mutex
	responsesWebSocketPools   = make(map[int]*responsesWebSocketPool)
)

type responsesWebSocketDialError struct {
	err      error
	response *http.Response
}

func (e *responsesWebSocketDialError) Error() string {
	if e == nil || e.err == nil {
		return "codex websocket dial failed"
	}
	return e.err.Error()
}

func (e *responsesWebSocketDialError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

type responsesWebSocketPool struct {
	channelID      int
	signature      string
	requestURL     string
	proxyURL       string
	headerTemplate http.Header

	mu        sync.Mutex
	available chan *responsesPooledConn
	all       map[*responsesPooledConn]struct{}
	total     int
	closed    bool
}

type responsesPooledConn struct {
	conn *websocket.Conn
	pool *responsesWebSocketPool
}

type borrowedResponsesWebSocketConn struct {
	pooled   *responsesPooledConn
	released atomic.Bool
}

func buildResponsesWebSocketPoolSignature(requestURL string, apiKey string, proxyURL string) string {
	sum := sha256.Sum256([]byte(requestURL + "\n" + apiKey + "\n" + proxyURL))
	return hex.EncodeToString(sum[:])
}

func getOrCreateResponsesWebSocketPool(channelID int, signature string, requestURL string, headerTemplate http.Header, proxyURL string) *responsesWebSocketPool {
	responsesWebSocketPoolsMu.Lock()
	defer responsesWebSocketPoolsMu.Unlock()

	if existing, ok := responsesWebSocketPools[channelID]; ok {
		if !existing.closed && existing.signature == signature {
			return existing
		}
		delete(responsesWebSocketPools, channelID)
		go existing.Close()
	}

	pool := &responsesWebSocketPool{
		channelID:      channelID,
		signature:      signature,
		requestURL:     requestURL,
		proxyURL:       proxyURL,
		headerTemplate: headerTemplate.Clone(),
		available:      make(chan *responsesPooledConn, maxResponsesWebSocketConnectionsPerChannel),
		all:            make(map[*responsesPooledConn]struct{}),
	}
	responsesWebSocketPools[channelID] = pool
	return pool
}

func CloseResponsesWebSocketPoolsForChannel(channelID int) {
	responsesWebSocketPoolsMu.Lock()
	pool, ok := responsesWebSocketPools[channelID]
	if ok {
		delete(responsesWebSocketPools, channelID)
	}
	responsesWebSocketPoolsMu.Unlock()

	if ok {
		pool.Close()
	}
}

func (p *responsesWebSocketPool) Acquire(ctx context.Context) (*borrowedResponsesWebSocketConn, error) {
	if p == nil {
		return nil, errResponsesWebSocketPoolClosed
	}

	for {
		select {
		case pooledConn := <-p.available:
			if pooledConn == nil {
				continue
			}
			return &borrowedResponsesWebSocketConn{pooled: pooledConn}, nil
		default:
		}

		pooledConn, err, created := p.tryCreateConn(ctx)
		if created {
			if err != nil {
				return nil, err
			}
			return &borrowedResponsesWebSocketConn{pooled: pooledConn}, nil
		}

		select {
		case pooledConn := <-p.available:
			if pooledConn == nil {
				continue
			}
			return &borrowedResponsesWebSocketConn{pooled: pooledConn}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (p *responsesWebSocketPool) tryCreateConn(ctx context.Context) (*responsesPooledConn, error, bool) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errResponsesWebSocketPoolClosed, true
	}
	if p.total >= maxResponsesWebSocketConnectionsPerChannel {
		p.mu.Unlock()
		return nil, nil, false
	}
	p.total++
	p.mu.Unlock()

	pooledConn, err := p.createConn(ctx)
	if err != nil {
		p.mu.Lock()
		if p.total > 0 {
			p.total--
		}
		p.mu.Unlock()
		return nil, err, true
	}

	p.mu.Lock()
	if p.closed {
		if p.total > 0 {
			p.total--
		}
		p.mu.Unlock()
		_ = pooledConn.conn.Close()
		return nil, errResponsesWebSocketPoolClosed, true
	}
	p.all[pooledConn] = struct{}{}
	p.mu.Unlock()

	return pooledConn, nil, true
}

func (p *responsesWebSocketPool) createConn(ctx context.Context) (*responsesPooledConn, error) {
	dialer, err := service.NewWebSocketDialer(p.proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create websocket dialer failed: %w", err)
	}

	targetHeader := p.headerTemplate.Clone()
	sessionID := common.GetUUID()
	targetHeader.Set("session_id", sessionID)
	targetHeader.Set("x-client-request-id", sessionID)
	targetHeader.Set("x-codex-turn-metadata", fmt.Sprintf(`{"session_id":"%s","turn_id":"%s","sandbox":"seatbelt"}`, sessionID, sessionID))

	targetConn, resp, err := dialer.DialContext(ctx, p.requestURL, targetHeader)
	if err != nil {
		return nil, &responsesWebSocketDialError{
			err:      err,
			response: resp,
		}
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	return &responsesPooledConn{
		conn: targetConn,
		pool: p,
	}, nil
}

func (p *responsesWebSocketPool) release(pooledConn *responsesPooledConn, discard bool) {
	if p == nil || pooledConn == nil {
		return
	}

	p.mu.Lock()
	_, exists := p.all[pooledConn]
	if !exists {
		closed := p.closed
		p.mu.Unlock()
		if closed || discard {
			_ = pooledConn.conn.Close()
		}
		return
	}

	if discard || p.closed {
		delete(p.all, pooledConn)
		if p.total > 0 {
			p.total--
		}
		p.mu.Unlock()
		_ = pooledConn.conn.Close()
		return
	}
	p.mu.Unlock()

	select {
	case p.available <- pooledConn:
	default:
		p.release(pooledConn, true)
	}
}

func (p *responsesWebSocketPool) Close() {
	if p == nil {
		return
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true

	allConns := make([]*responsesPooledConn, 0, len(p.all))
	for pooledConn := range p.all {
		allConns = append(allConns, pooledConn)
	}
	p.all = make(map[*responsesPooledConn]struct{})
	p.total = 0
	p.mu.Unlock()

	for _, pooledConn := range allConns {
		_ = pooledConn.conn.Close()
	}

	for {
		select {
		case <-p.available:
		default:
			return
		}
	}
}

func (b *borrowedResponsesWebSocketConn) Conn() *websocket.Conn {
	if b == nil || b.pooled == nil {
		return nil
	}
	return b.pooled.conn
}

func (b *borrowedResponsesWebSocketConn) Release() {
	if b == nil || b.pooled == nil || !b.released.CompareAndSwap(false, true) {
		return
	}
	b.pooled.pool.release(b.pooled, false)
}

func (b *borrowedResponsesWebSocketConn) Discard() {
	if b == nil || b.pooled == nil || !b.released.CompareAndSwap(false, true) {
		return
	}
	b.pooled.pool.release(b.pooled, true)
}
