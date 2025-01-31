package fsutil

import "path/filepath"

type DiffType int

const (
	DiffOpAdd DiffType = iota
	DiffOpDel
)

type DiffOp struct {
	Type DiffType
	Path string
}

func (o DiffOp) String() string {
	var s string
	switch o.Type {
	case DiffOpAdd:
		s = "A"
	case DiffOpDel:
		s = "D"
	default:
		s = "?"
	}
	return s + " " + o.Path
}

// FSDiff returns the operations that needs to be done on n2 to make it look like n1.
func FSDiff(n1, n2 FSNode) []DiffOp {
	return fsDiffInner(n1, n2, ".")
}

func fsDiffInner(n1, n2 FSNode, base string) []DiffOp {
	var ops []DiffOp
	ln := n1.Nodes
	rn := n2.Nodes
	for len(ln) > 0 && len(rn) > 0 {
		l, r := ln[0], rn[0]

		if l.Name < r.Name { // file doesn't exist in r
			ops = append(ops, DiffOp{Type: DiffOpAdd, Path: canonicalPath(base, l.Name)})
			ln = ln[1:] // advance
		} else if l.Name > r.Name { // file doesn't exist in l
			ops = append(ops, DiffOp{Type: DiffOpDel, Path: canonicalPath(base, r.Name)})
			rn = rn[1:]
		} else { // l.Name == r.Name (same item)
			if l.Mode.IsDir() != r.Mode.IsDir() { // one of them is a directory
				ops = append(ops, DiffOp{Type: DiffOpDel, Path: canonicalPath(base, l.Name)})
				ops = append(ops, DiffOp{Type: DiffOpAdd, Path: canonicalPath(base, l.Name)})
			} else if l.checksum() != r.checksum() {
				if !l.Mode.IsDir() && !r.Mode.IsDir() {
					// Nodes are not dir, re-upload file
					ops = append(ops, DiffOp{Type: DiffOpAdd, Path: canonicalPath(base, l.Name)})
				} else {
					// both Nodes are dir, recurse:
					ops = append(ops, fsDiffInner(l, r, canonicalPath(base, l.Name))...)
				}
			}
			ln, rn = ln[1:], rn[1:]
		}
	}
	// add remaining
	for _, l := range ln {
		ops = append(ops, DiffOp{Type: DiffOpAdd, Path: canonicalPath(base, l.Name)})
	}
	for _, r := range rn {
		ops = append(ops, DiffOp{Type: DiffOpDel, Path: canonicalPath(base, r.Name)})
	}
	return ops
}

// canonicalPath joins base and rel to create a canonical path string with unix path separator (/) independent of
// current platform.
func canonicalPath(base, rel string) string {
	return filepath.ToSlash(filepath.Join(base, rel))
}
