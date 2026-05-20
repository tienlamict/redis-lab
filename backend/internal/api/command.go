// Execute a free-form redis command against a given lab. For cluster mode
// we surface the slot + redirect chain in addition to the result.
package api

import (
	"context"
	"encoding/csv"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type commandReq struct {
	Command string `json:"command"`
}

func (d *Deps) handleCommand(c *gin.Context) {
	lab := c.Param("lab")
	var body commandReq
	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	args, err := splitCommand(body.Command)
	if err != nil || len(args) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty or unparseable command"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5e9)
	defer cancel()

	switch lab {
	case "standalone":
		res, err := d.Standalone.Cmd(ctx, args...)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": formatResult(res)})
	case "sentinel":
		res, err := d.Sentinel.Cmd(ctx, args...)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": formatResult(res)})
	case "cluster":
		out, err := d.Cluster.ExecuteWithRedirectLog(ctx, args)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error(), "redirects": out.Redirects, "slot": out.Slot})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"result":     formatResult(out.Result),
			"executedOn": out.ExecutedOn,
			"slot":       out.Slot,
			"redirects":  out.Redirects,
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown lab"})
	}
}

// splitCommand handles simple shell-style quoting via encoding/csv with space
// as delimiter. Good enough for demo, NOT a full shell parser.
func splitCommand(s string) ([]any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ' '
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	rec, err := r.Read()
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rec))
	for _, f := range rec {
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// formatResult coerces go-redis return shapes into JSON-friendly values.
func formatResult(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = formatResult(x)
		}
		return out
	default:
		return v
	}
}
