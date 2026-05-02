package api

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/ngate/internal/models"
)

// CertLogBroker is an in-memory pub/sub for certificate issuance log lines.
// Each cert ID maps to a list of subscriber channels. Sends are non-blocking
// so a slow/dropped client never blocks the issuer goroutine.
type CertLogBroker struct {
	mu          sync.Mutex
	subscribers map[int64][]chan string
}

func NewCertLogBroker() *CertLogBroker {
	return &CertLogBroker{subscribers: make(map[int64][]chan string)}
}

// CreateStream initialises the subscriber list for a cert ID.
func (b *CertLogBroker) CreateStream(certID int64) {
	b.mu.Lock()
	b.subscribers[certID] = nil
	b.mu.Unlock()
}

// Send broadcasts a line to all subscribers of certID. Non-blocking.
func (b *CertLogBroker) Send(certID int64, line string) {
	b.mu.Lock()
	subs := b.subscribers[certID]
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- line:
		default:
			// drop for slow consumers
		}
	}
}

// CloseStream closes all subscriber channels and removes the entry.
func (b *CertLogBroker) CloseStream(certID int64) {
	b.mu.Lock()
	for _, ch := range b.subscribers[certID] {
		close(ch)
	}
	delete(b.subscribers, certID)
	b.mu.Unlock()
}

// Subscribe returns a buffered channel that receives log lines for certID.
func (b *CertLogBroker) Subscribe(certID int64) chan string {
	ch := make(chan string, 64)
	b.mu.Lock()
	b.subscribers[certID] = append(b.subscribers[certID], ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel from certID.
func (b *CertLogBroker) Unsubscribe(certID int64, ch chan string) {
	b.mu.Lock()
	subs := b.subscribers[certID]
	for i, s := range subs {
		if s == ch {
			b.subscribers[certID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	b.mu.Unlock()
}

func (h *Handler) serveCertLogs(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	cert, err := h.db.GetCertificate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Certificate not found"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)

	// Replay persisted history first.
	if f, err := os.Open(h.certs.LogPath(id)); err == nil {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			fmt.Fprintf(c.Writer, "data: %s\n\n", scanner.Text())
		}
		f.Close()
		c.Writer.Flush()
	}

	// If no issuance is in progress, end the stream — client gets history only.
	if cert.Status != models.CertStatusIssuing {
		fmt.Fprintf(c.Writer, "event: done\ndata: \n\n")
		return
	}

	ch := h.certLogs.Subscribe(id)
	defer h.certLogs.Unsubscribe(id, ch)

	clientGone := c.Request.Context().Done()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientGone:
			return false
		case line, ok := <-ch:
			if !ok {
				fmt.Fprintf(w, "event: done\ndata: \n\n")
				return false
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			return true
		}
	})
}
