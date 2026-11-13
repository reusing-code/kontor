package quickadd

import (
	"bytes"
	"errors"
	"strings"

	"github.com/ledongthuc/pdf"
)

const (
	maxPDFPages     = 30
	maxPDFOperators = 2_000_000
	maxPDFTextBytes = 100_000
	// A horizontal gap beyond this (in thousandths of the font size) is a
	// word gap; kerning stays well below it.
	pdfWordGap = 180
)

var errPDFUnreadable = errors.New("quickadd: unreadable pdf")

// errPDFLimit stops the content stream interpreter once enough was read.
var errPDFLimit = errors.New("quickadd: pdf limit reached")

// pdfText returns the text layer of a PDF, empty for a scan. The parser
// panics on malformed files, which is reported as errPDFUnreadable.
func pdfText(data []byte) (text string, err error) {
	defer func() {
		if recover() != nil {
			text, err = "", errPDFUnreadable
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errPDFUnreadable
	}
	ex := &pdfExtractor{}
	for i := 1; i <= min(r.NumPage(), maxPDFPages) && !ex.done; i++ {
		ex.page(r.Page(i))
	}
	return normalizeLines(ex.out.String()), nil
}

type pdfExtractor struct {
	out       strings.Builder
	operators int
	done      bool
}

func (ex *pdfExtractor) page(p pdf.Page) {
	if p.V.IsNull() || p.V.Key("Contents").Kind() == pdf.Null {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			if r != errPDFLimit {
				panic(r)
			}
			ex.done = true
		}
	}()

	fonts := map[string]*pdfFont{}
	var (
		font *pdfFont
		// advance is the width of the text shown since the last move, in
		// thousandths of the font size.
		advance  float64
		fontSize float64
		lineY    float64
	)
	show := func(raw string) {
		if font == nil {
			return
		}
		ex.out.WriteString(font.enc.Decode(raw))
		advance += font.width(raw)
	}
	newline := func() {
		ex.out.WriteByte('\n')
		advance = 0
	}

	pdf.Interpret(p.V.Key("Contents"), func(stk *pdf.Stack, op string) {
		if ex.operators++; ex.operators > maxPDFOperators || ex.out.Len() > maxPDFTextBytes {
			panic(errPDFLimit)
		}
		args := make([]pdf.Value, stk.Len())
		for i := len(args) - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}
		switch op {
		case "BT", "T*":
			newline()
		case "Td", "TD":
			switch {
			case len(args) != 2:
			case args[1].Float64() != 0:
				newline()
			default:
				// A move along the line: kerning, or a jump to e.g. a
				// right-aligned amount.
				if fontSize > 0 && args[0].Float64()*1000/fontSize-advance > pdfWordGap {
					ex.out.WriteByte(' ')
				}
				advance = 0
			}
		case "Tm":
			if len(args) == 6 {
				if y := args[5].Float64(); y != lineY {
					lineY = y
					newline()
				}
			}
		case "Tf":
			if len(args) == 2 {
				name := args[0].Name()
				if _, ok := fonts[name]; !ok {
					fonts[name] = newPDFFont(p.Font(name))
				}
				font, fontSize = fonts[name], args[1].Float64()
			}
		case "'", "\"":
			newline()
			if len(args) > 0 {
				show(args[len(args)-1].RawString())
			}
		case "Tj":
			if len(args) == 1 {
				show(args[0].RawString())
			}
		case "TJ":
			if len(args) != 1 {
				return
			}
			for i := range args[0].Len() {
				switch x := args[0].Index(i); x.Kind() {
				case pdf.String:
					show(x.RawString())
				case pdf.Integer, pdf.Real:
					if x.Float64() < -pdfWordGap {
						ex.out.WriteByte(' ')
					}
					advance -= x.Float64()
				}
			}
		}
	})
}

// pdfFont decodes the strings of one font and measures them.
type pdfFont struct {
	enc pdf.TextEncoding
	// simple is set for single-byte fonts; composite fonts use two-byte
	// codes with the widths below.
	simple       *pdf.Font
	widths       map[int]float64
	defaultWidth float64
}

func newPDFFont(f pdf.Font) *pdfFont {
	if f.V.IsNull() {
		return nil
	}
	font := &pdfFont{enc: f.Encoder()}
	if f.V.Key("Subtype").Name() != "Type0" {
		font.simple = &f
		return font
	}
	desc := f.V.Key("DescendantFonts").Index(0)
	font.defaultWidth = 1000
	if dw := desc.Key("DW"); dw.Kind() != pdf.Null {
		font.defaultWidth = dw.Float64()
	}
	// W is a list of "first [w1 w2 ...]" and "first last w" entries.
	font.widths = map[int]float64{}
	w := desc.Key("W")
	for i := 0; i+1 < w.Len(); {
		first := int(w.Index(i).Int64())
		if list := w.Index(i + 1); list.Kind() == pdf.Array {
			for j := range min(list.Len(), 1<<16) {
				font.widths[first+j] = list.Index(j).Float64()
			}
			i += 2
			continue
		}
		if i+2 >= w.Len() {
			break
		}
		last := min(int(w.Index(i+1).Int64()), first+1<<16)
		for code := first; code <= last; code++ {
			font.widths[code] = w.Index(i + 2).Float64()
		}
		i += 3
	}
	return font
}

// width is the advance of raw in thousandths of the font size. Character
// and word spacing are ignored.
func (f *pdfFont) width(raw string) float64 {
	var total float64
	if f.simple != nil {
		for i := range len(raw) {
			total += f.simple.Width(int(raw[i]))
		}
		return total
	}
	for i := 0; i+1 < len(raw); i += 2 {
		if w, ok := f.widths[int(raw[i])<<8|int(raw[i+1])]; ok {
			total += w
		} else {
			total += f.defaultWidth
		}
	}
	return total
}

// normalizeLines trims every line, collapses runs of spaces and drops empty
// lines.
func normalizeLines(s string) string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
