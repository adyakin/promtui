// Package kitty places PNGs using the classic Kitty graphics protocol.
package kitty

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const BaseID = 0x507455
const ProbeID = BaseID + 2

// Query a 1×1 RGB pixel without creating a placement.
func Probe() string { return fmt.Sprintf("\x1b_Ga=q,i=%d,t=d,f=24,s=1,v=1;AAAA\x1b\\", ProbeID) }

func Delete(id int) string { return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id) }
func Clear() string        { return Delete(BaseID) + Delete(BaseID+1) }
func Frame(png []byte, id, row, cols, rows int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\x1b7\x1b[%d;1H", row)
	data := base64.StdEncoding.EncodeToString(png)
	for off := 0; off < len(data); off += 4096 {
		end := min(off+4096, len(data))
		m := 0
		if end < len(data) {
			m = 1
		}
		b.WriteString("\x1b_G")
		if off == 0 {
			fmt.Fprintf(&b, "a=T,f=100,t=d,i=%d,p=1,c=%d,r=%d,C=1,q=2,", id, cols, rows)
		}
		fmt.Fprintf(&b, "m=%d;%s\x1b\\", m, data[off:end])
	}
	b.WriteString(Delete(BaseID + (1 - (id - BaseID))))
	b.WriteString("\x1b8")
	return b.String()
}
