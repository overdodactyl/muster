package render

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func JSON(w io.Writer, v any) error {
	if w == nil {
		w = os.Stdout
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
