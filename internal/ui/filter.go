// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/docker"
)

type filterKind int

const (
	filterText filterKind = iota
	filterName
	filterID
	filterStatus
	filterLabel
	filterImage
	filterPort
	filterIs
)

type filterPredicate struct {
	kind    filterKind
	val     string
	key     string // for label:key=value
	hasKey  bool
	negated bool
}

type filterQuery struct {
	raw        string
	predicates []filterPredicate
}

// parseFilter parses a filter query string into a structured filterQuery.
// It supports:
//   - Multiple terms separated by space (conjunction / AND)
//   - Quoted phrases ("exact phrase" or 'exact phrase')
//   - Negation with "!" or "-" (e.g. !exited, -prod)
//   - Prefixed filters:
//     name:<val>, n:<val>
//     id:<val>
//     status:<val>, state:<val>
//     label:<key>, label:<key>=<val>, l:<key>=<val>
//     image:<val>, img:<val>
//     port:<val>, p:<val>
//     is:<running|stopped|exited|paused|healthy|unhealthy|dangling>
func parseFilter(raw string) filterQuery {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return filterQuery{}
	}

	tokens := tokenizeFilter(raw)
	preds := make([]filterPredicate, 0, len(tokens))

	for _, token := range tokens {
		p := parseToken(token)
		if p != nil {
			preds = append(preds, *p)
		}
	}

	return filterQuery{raw: raw, predicates: preds}
}

func tokenizeFilter(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := rune(0)

	for _, r := range s {
		switch {
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = r
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func parseToken(token string) *filterPredicate {
	if token == "" {
		return nil
	}

	negated := false
	if strings.HasPrefix(token, "!") {
		negated = true
		token = token[1:]
	} else if strings.HasPrefix(token, "-") && len(token) > 1 {
		negated = true
		token = token[1:]
	}

	if token == "" {
		return nil
	}

	var (
		kind   filterKind
		val    string
		key    string
		hasKey bool
	)

	if idx := strings.Index(token, ":"); idx > 0 {
		prefix := strings.ToLower(token[:idx])
		rem := token[idx+1:]

		switch prefix {
		case "is":
			kind = filterIs
			val = strings.ToLower(rem)
		case "status", "state":
			kind = filterStatus
			val = strings.ToLower(rem)
		case "label", "l":
			kind = filterLabel
			if k, v, ok := strings.Cut(rem, "="); ok {
				key = strings.ToLower(k)
				val = strings.ToLower(v)
				hasKey = true
			} else {
				key = strings.ToLower(rem)
				val = ""
				hasKey = false
			}
		case "name", "n":
			kind = filterName
			val = strings.ToLower(rem)
		case "id":
			kind = filterID
			val = strings.ToLower(rem)
		case "image", "img":
			kind = filterImage
			val = strings.ToLower(rem)
		case "port", "p":
			kind = filterPort
			val = strings.ToLower(rem)
		default:
			// Unrecognized prefix (e.g. "nginx:alpine", "localhost:8080") -> plain text search
			kind = filterText
			val = strings.ToLower(token)
		}
	} else {
		kind = filterText
		val = strings.ToLower(token)
	}

	return &filterPredicate{
		kind:    kind,
		val:     val,
		key:     key,
		hasKey:  hasKey,
		negated: negated,
	}
}

// matches reports whether an item matches all predicates in the query.
func (q filterQuery) matches(it item) bool {
	if len(q.predicates) == 0 {
		return true
	}
	for _, p := range q.predicates {
		matched := evalPredicate(it, p)
		if p.negated {
			matched = !matched
		}
		if !matched {
			return false
		}
	}
	return true
}

func evalPredicate(it item, p filterPredicate) bool {
	switch p.kind {
	case filterName:
		if strings.Contains(strings.ToLower(it.name), p.val) {
			return true
		}
		if c, ok := it.data.(docker.Container); ok {
			for _, n := range c.Names {
				if strings.Contains(strings.ToLower(n), p.val) {
					return true
				}
			}
		}
		// Also check the name column in cells if present (typically column 1 or 0)
		for _, idx := range []int{0, 1} {
			if idx < len(it.cells) && strings.Contains(strings.ToLower(it.cells[idx]), p.val) {
				return true
			}
		}
		return false

	case filterID:
		if strings.Contains(strings.ToLower(it.id), p.val) {
			return true
		}
		if len(it.cells) > 0 && strings.Contains(strings.ToLower(it.cells[0]), p.val) {
			return true
		}
		return false

	case filterStatus:
		if c, ok := it.data.(docker.Container); ok {
			if strings.Contains(strings.ToLower(c.State), p.val) || strings.Contains(strings.ToLower(c.Status), p.val) {
				return true
			}
		}
		if proj, ok := it.data.(buildx.Project); ok {
			if strings.Contains(strings.ToLower(proj.Status), p.val) {
				return true
			}
		}
		if rec, ok := it.data.(buildx.Record); ok {
			if strings.Contains(strings.ToLower(rec.Status), p.val) {
				return true
			}
		}
		// Fallback: search all cells for status value
		for _, c := range it.cells {
			if strings.Contains(strings.ToLower(c), p.val) {
				return true
			}
		}
		return false

	case filterLabel:
		labels := itemLabels(it)
		if len(labels) == 0 {
			return false
		}
		if p.hasKey {
			for k, v := range labels {
				if (strings.EqualFold(k, p.key) || strings.Contains(strings.ToLower(k), p.key)) &&
					strings.Contains(strings.ToLower(v), p.val) {
					return true
				}
			}
			return false
		}
		// Match either label key or label value
		for k, v := range labels {
			if strings.Contains(strings.ToLower(k), p.key) || strings.Contains(strings.ToLower(v), p.key) {
				return true
			}
		}
		return false

	case filterImage:
		if c, ok := it.data.(docker.Container); ok {
			if strings.Contains(strings.ToLower(c.Image), p.val) {
				return true
			}
		}
		if img, ok := it.data.(docker.Image); ok {
			if strings.Contains(strings.ToLower(img.Tag()), p.val) {
				return true
			}
			for _, tag := range img.RepoTags {
				if strings.Contains(strings.ToLower(tag), p.val) {
					return true
				}
			}
		}
		// Check cells where image is displayed (typically column 3 for containers)
		if len(it.cells) > 3 && strings.Contains(strings.ToLower(it.cells[3]), p.val) {
			return true
		}
		return false

	case filterPort:
		if c, ok := it.data.(docker.Container); ok {
			for _, port := range c.Ports {
				if strings.Contains(fmt.Sprint(port.PrivatePort), p.val) ||
					(port.PublicPort > 0 && strings.Contains(fmt.Sprint(port.PublicPort), p.val)) ||
					strings.Contains(strings.ToLower(port.Type), p.val) {
					return true
				}
			}
		}
		// Check port cell (last cell in containers tab)
		if len(it.cells) > 0 && strings.Contains(strings.ToLower(it.cells[len(it.cells)-1]), p.val) {
			return true
		}
		return false

	case filterIs:
		return evalIs(it, p.val)

	default: // filterText
		if strings.Contains(strings.ToLower(it.name), p.val) {
			return true
		}
		if strings.Contains(strings.ToLower(it.id), p.val) {
			return true
		}
		for _, c := range it.cells {
			if strings.Contains(strings.ToLower(c), p.val) {
				return true
			}
		}
		return false
	}
}

func evalIs(it item, val string) bool {
	c, isContainer := it.data.(docker.Container)
	switch val {
	case "running":
		if isContainer {
			return strings.EqualFold(c.State, "running")
		}
		if p, ok := it.data.(buildx.Project); ok {
			return strings.Contains(strings.ToLower(p.Status), "running")
		}
		for _, cell := range it.cells {
			low := strings.ToLower(cell)
			if low == "running" || strings.HasPrefix(low, "up ") {
				return true
			}
		}
		return false

	case "stopped", "exited":
		if isContainer {
			return c.State != "" && !strings.EqualFold(c.State, "running")
		}
		for _, cell := range it.cells {
			low := strings.ToLower(cell)
			if strings.HasPrefix(low, "exited") || low == "stopped" || low == "dead" || low == "created" {
				return true
			}
		}
		return false

	case "paused":
		if isContainer {
			return strings.EqualFold(c.State, "paused")
		}
		for _, cell := range it.cells {
			if strings.Contains(strings.ToLower(cell), "paused") {
				return true
			}
		}
		return false

	case "healthy":
		if isContainer {
			low := strings.ToLower(c.Status)
			return strings.Contains(low, "healthy") && !strings.Contains(low, "unhealthy")
		}
		for _, cell := range it.cells {
			low := strings.ToLower(cell)
			if strings.Contains(low, "healthy") && !strings.Contains(low, "unhealthy") {
				return true
			}
		}
		return false

	case "unhealthy":
		if isContainer {
			return strings.Contains(strings.ToLower(c.Status), "unhealthy")
		}
		for _, cell := range it.cells {
			if strings.Contains(strings.ToLower(cell), "unhealthy") {
				return true
			}
		}
		return false

	case "dangling", "unused":
		if img, ok := it.data.(docker.Image); ok {
			return img.Containers == 0 || strings.Contains(img.Tag(), "<none>")
		}
		if _, ok := it.data.(docker.Volume); ok {
			if len(it.cells) > 0 {
				last := it.cells[len(it.cells)-1]
				return last == "-" || last == "0"
			}
		}
		return false

	default:
		// Unknown is:<val>, check if any state/status or cell matches <val>
		return evalPredicate(it, filterPredicate{kind: filterStatus, val: val})
	}
}

func itemLabels(it item) map[string]string {
	switch d := it.data.(type) {
	case docker.Container:
		return d.Labels
	case docker.Image:
		return d.Labels
	case docker.Volume:
		return d.Labels
	case docker.Network:
		return d.Labels
	default:
		return nil
	}
}
