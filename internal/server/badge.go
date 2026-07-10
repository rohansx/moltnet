package server

import (
	"fmt"

	"github.com/moltnet/moltnet/score"
)

// tierFor maps a score to a display tier and accent colour.
func tierFor(s float64) (label, color string) {
	switch {
	case s >= 85:
		return "elite", "#22c55e"
	case s >= 70:
		return "trusted", "#3b82f6"
	case s >= 50:
		return "established", "#a855f7"
	case s >= 30:
		return "emerging", "#eab308"
	default:
		return "new", "#94a3b8"
	}
}

// badgeSVG renders an embeddable MoltScore badge (README/landing style, like a
// CI or npm badge). Left cell is the label, right cell is the score + tier.
func badgeSVG(name string, out score.Output) string {
	_, color := tierFor(out.Score)
	right := fmt.Sprintf("%.1f · %d✓", out.Score, out.Inputs.Completions)

	const (
		h        = 20
		labelTxt = "moltscore"
	)
	labelW := 6*len(labelTxt) + 12
	valueW := 6*len(right) + 14
	total := labelW + valueW

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" role="img" aria-label="moltscore: %s">
  <linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="%d" height="%d" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="%d" fill="#1f2328"/>
    <rect x="%d" width="%d" height="%d" fill="%s"/>
    <rect width="%d" height="%d" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
    <text x="%d" y="14">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`,
		total, h, right,
		total, h,
		labelW, h,
		labelW, valueW, h, color,
		total, h,
		labelW/2, labelTxt,
		labelW+valueW/2, right,
	)
}
