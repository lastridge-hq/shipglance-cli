// Package render turns a query.Result into output: the --json envelope that
// agents parse, or readable text for a terminal. It writes only to the
// writers it is given.
package render

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/lastridge-hq/shipglance-cli/internal/outcome"
	"github.com/lastridge-hq/shipglance-cli/internal/query"
)

// Envelope is the one --json shape (docs/cli-plan.md §5): the CLI owns the
// envelope and the closed `outcome`; `data` is the /api/v1 body verbatim (a
// success or the server's typed error), or a CLI {kind, message} when no
// server body exists. `notice` is the update notice, null when current.
type Envelope struct {
	Outcome outcome.Outcome `json:"outcome"`
	Notice  *query.Notice   `json:"notice"`
	Data    json.RawMessage `json:"data"`
}

// JSON writes the envelope as one line.
func JSON(w io.Writer, result query.Result) error {
	data := result.Data
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	compact, err := compactJSON(data)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(Envelope{Outcome: result.Outcome, Notice: result.Notice, Data: compact})
	if err != nil {
		return err
	}
	_, err = w.Write(append(encoded, '\n'))
	return err
}

// compactJSON re-encodes data on one line without changing its content, so
// the envelope stays one line whatever whitespace the server used.
func compactJSON(raw []byte) (json.RawMessage, error) {
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
