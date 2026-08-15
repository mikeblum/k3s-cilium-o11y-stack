package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

type table struct {
	w     *tabwriter.Writer
	title string
	cols  int
}

func newTable(title string, headers ...string) *table {
	fmt.Printf("== %s ==\n", title)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	t := &table{w: w, title: title, cols: len(headers)}
	fmt.Fprintln(w, joinTab(headers))
	return t
}

func (t *table) row(cells ...string) {
	fmt.Fprintln(t.w, joinTab(cells))
}

func (t *table) flush(nRows int, total string) {
	if nRows == 0 {
		fmt.Fprintln(t.w, joinTab(pad([]string{"(no data)"}, t.cols)))
	} else {
		cells := make([]string, t.cols)
		cells[0] = "TOTAL"
		cells[t.cols-1] = total
		fmt.Fprintln(t.w, joinTab(cells))
	}
	t.w.Flush()
}

func joinTab(cells []string) string {
	return strings.Join(cells, "\t")
}

func pad(cells []string, n int) []string {
	for len(cells) < n {
		cells = append(cells, "")
	}
	return cells
}

func comma(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := ""
	if n < 0 {
		neg, s = "-", s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return neg + string(out)
}
