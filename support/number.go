package support

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Num provides number formatting helpers, Laravel's Number.
var Num = &NumberHelper{}

// NumberHelper renders numbers for people to read.
type NumberHelper struct{}

// Format renders an integer with thousands separators: 1234 -> "1,234".
func (n *NumberHelper) Format(value int64) string {
	rendered := strconv.FormatInt(value, 10)

	negative := strings.HasPrefix(rendered, "-")
	rendered = strings.TrimPrefix(rendered, "-")

	var grouped strings.Builder
	for i, digit := range rendered {
		if i > 0 && (len(rendered)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}

	if negative {
		return "-" + grouped.String()
	}
	return grouped.String()
}

// FormatFloat renders a float with thousands separators and a fixed
// number of decimals.
func (n *NumberHelper) FormatFloat(value float64, decimals int) string {
	rendered := strconv.FormatFloat(roundHalfAway(value, decimals), 'f', decimals, 64)

	whole, fraction, _ := strings.Cut(rendered, ".")

	parsed, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return rendered
	}
	grouped := n.Format(parsed)

	if fraction == "" {
		return grouped
	}
	return grouped + "." + fraction
}

// Currency renders an amount with a symbol: 1234.56 -> "$1,234.56".
func (n *NumberHelper) Currency(value float64, symbol string, decimals ...int) string {
	places := 2
	if len(decimals) > 0 {
		places = decimals[0]
	}
	return symbol + n.FormatFloat(value, places)
}

// Percentage renders a percentage: 12.5 -> "12.5%".
func (n *NumberHelper) Percentage(value float64, decimals ...int) string {
	places := 1
	if len(decimals) > 0 {
		places = decimals[0]
	}
	return strconv.FormatFloat(roundHalfAway(value, places), 'f', places, 64) + "%"
}

// roundHalfAway rounds a half up in magnitude, which is what a reader
// expects from a displayed number: 12.5% shown to no decimals is 13%,
// not the 12% Go's default round-half-to-even would print.
func roundHalfAway(value float64, decimals int) float64 {
	shift := math.Pow(10, float64(decimals))
	return math.Round(value*shift) / shift
}

// fileSizeUnits are the binary units FileSize steps through.
var fileSizeUnits = []string{"KB", "MB", "GB", "TB", "PB"}

// FileSize renders a byte count: 1024 -> "1.0 KB".
func (n *NumberHelper) FileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}

	size := float64(bytes)
	unit := ""
	for _, candidate := range fileSizeUnits {
		size /= 1024
		unit = candidate
		if size < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", size, unit)
}

// Ordinal renders a position: 1 -> "1st", 11 -> "11th".
func (n *NumberHelper) Ordinal(value int) string {
	suffix := "th"

	// The teens all take "th", which is the case a naive implementation
	// gets wrong: 11th, not 11st.
	if remainder := value % 100; remainder < 11 || remainder > 13 {
		switch value % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}

	return strconv.Itoa(value) + suffix
}

// Clamp confines a value to a range. It is a function rather than a
// method on Num because Go methods cannot take type parameters.
func Clamp[T int | int64 | float64](value, low, high T) T {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
