package agent

import (
	"fmt"
	"strings"
)

// A managed section is a single contiguous edit; common prefix/suffix trimming
// produces a compact unified hunk without invoking an external diff program.
func unifiedDiff(path, before, after string, created bool) string {
	if before == after {
		return ""
	}
	lines := func(text string) []string {
		if text == "" {
			return nil
		}
		parts := strings.SplitAfter(text, "\n")
		if parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
		return parts
	}
	a, b := lines(before), lines(after)
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	start := max(0, prefix-3)
	endA, endB := min(len(a), len(a)-suffix+3), min(len(b), len(b)-suffix+3)
	rangeText := func(start, count int) string {
		if count == 0 {
			return fmt.Sprintf("%d,0", start)
		}
		if count == 1 {
			return fmt.Sprint(start + 1)
		}
		return fmt.Sprintf("%d,%d", start+1, count)
	}
	var out strings.Builder
	from := "a/" + path
	if created {
		from = "/dev/null"
	}
	fmt.Fprintf(&out, "--- %s\n+++ b/%s\n@@ -%s +%s @@\n", from, path, rangeText(start, endA-start), rangeText(start, endB-start))
	write := func(prefix, line string) {
		out.WriteString(prefix + line)
		if !strings.HasSuffix(line, "\n") {
			out.WriteString("\n\\ No newline at end of file\n")
		}
	}
	for i := start; i < prefix; i++ {
		write(" ", a[i])
	}
	for i := prefix; i < len(a)-suffix; i++ {
		write("-", a[i])
	}
	for i := prefix; i < len(b)-suffix; i++ {
		write("+", b[i])
	}
	for i := len(a) - suffix; i < endA; i++ {
		write(" ", a[i])
	}
	return out.String()
}
