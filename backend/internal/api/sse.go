// SSE endpoint. One stream per lab. Subscribers get every event the
// broadcaster has for that lab; slow clients lose events silently (per
// broadcaster contract).
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (d *Deps) handleSSE(c *gin.Context) {
	lab := c.Param("lab")
	if d.nsForLab(lab) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown lab"})
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	ch, unsub := d.Broadcaster.Subscribe(lab)
	defer unsub()

	// Initial hello so clients know the stream is alive.
	fmt.Fprintf(c.Writer, "event: hello\ndata: {\"lab\":%q}\n\n", lab)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", ev.Type, b)
			flusher.Flush()
		case <-ticker.C:
			// keep-alive comment
			fmt.Fprint(c.Writer, ": ping\n\n")
			flusher.Flush()
		}
	}
}
