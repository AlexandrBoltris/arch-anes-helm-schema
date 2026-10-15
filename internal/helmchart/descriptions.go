package helmchart

// This file reads plain descriptions from values.yaml comments. It maps child
// chart comments into their dependency paths.

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
)

// loadDescriptions collects descriptions from the root chart and dependencies.
func loadDescriptions(ch *chartv2.Chart) (map[string]string, error) {
	descriptions := make(map[string]string)
	if err := appendChartDescriptions(ch, nil, descriptions); err != nil {
		return nil, err
	}
	return descriptions, nil
}

// appendChartDescriptions visits dependencies first. Parent comments can then
// replace child comments at the same effective values path.
func appendChartDescriptions(ch *chartv2.Chart, valuesPrefix []string, descriptions map[string]string) error {
	dependencies, err := bindDependencies(ch)
	if err != nil {
		return err
	}
	for _, dependency := range dependencies {
		childPrefix := appendValuePath(valuesPrefix, dependency.name)
		if err := appendChartDescriptions(dependency.chart, childPrefix, descriptions); err != nil {
			return err
		}
	}

	// Parent comments replace dependency comments because the parent owns the
	// final values path and can document its override more precisely.
	for _, file := range ch.Raw {
		if file == nil || file.Name != "values.yaml" {
			continue
		}
		if err := decodeDescriptions(file.Data, valuesPrefix, descriptions); err != nil {
			return err
		}
		break
	}
	return nil
}

// decodeDescriptions reads all YAML documents from one values.yaml file.
func decodeDescriptions(data []byte, valuesPrefix []string, descriptions map[string]string) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for documentNumber := 1; ; documentNumber++ {
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode YAML document %d: %w", documentNumber, err)
		}
		if len(document.Content) == 0 {
			continue
		}
		root := document.Content[0]
		source := newCommentSource(data)
		if description := normalizeComment(document.HeadComment, root.HeadComment, root.LineComment); description != "" {
			descriptions[jsonPointer(valuesPrefix)] = description
		}
		collectDescriptions(root, valuesPrefix, descriptions, source)
	}
}

// collectDescriptions walks mapping and sequence nodes and records comments by
// RFC 6901 JSON pointer.
func collectDescriptions(node *yaml.Node, valuePath []string, descriptions map[string]string, source *commentSource) {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			collectDescriptions(node.Content[0], valuePath, descriptions, source)
		}
	case yaml.MappingNode:
		previous := ""
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if key.Value == "<<" {
				continue
			}
			childPath := appendValuePath(valuePath, key.Value)
			head, indented := source.splitHeadComment(key)
			appendDescription(descriptions, previous, indented)
			if description := normalizeComment(head, key.LineComment, value.LineComment); description != "" {
				descriptions[jsonPointer(childPath)] = description
			}
			collectDescriptions(value, childPath, descriptions, source)
			previous = jsonPointer(childPath)
		}
	case yaml.SequenceNode:
		previous := ""
		for index, child := range node.Content {
			childPath := appendValuePath(valuePath, strconv.Itoa(index))
			head, indented := source.splitHeadComment(child)
			appendDescription(descriptions, previous, indented)
			if description := normalizeComment(head, child.LineComment); description != "" {
				descriptions[jsonPointer(childPath)] = description
			}
			collectDescriptions(child, childPath, descriptions, source)
			previous = jsonPointer(childPath)
		}
	case yaml.AliasNode:
		// Alias comments describe the alias location, not the anchor's original
		// path. Do not copy comments from the target into another value path.
		return
	}
}

// jsonPointer encodes a value path as an RFC 6901 JSON pointer.
func jsonPointer(valuePath []string) string {
	if len(valuePath) == 0 {
		return ""
	}
	escaped := make([]string, len(valuePath))
	for index, part := range valuePath {
		part = strings.ReplaceAll(part, "~", "~0")
		escaped[index] = strings.ReplaceAll(part, "/", "~1")
	}
	return "/" + strings.Join(escaped, "/")
}

// toolDirectives are comment prefixes that address an editor or another tool
// instead of documenting a value. A schema generator must drop the first one
// in particular, because it names the file that this command writes.
var toolDirectives = []string{
	"yaml-language-server:",
	"vim:",
	"vi:",
	"-*-",
}

// normalizeComment removes YAML and helm-docs markers. It also removes empty
// lines, documentation directives, and directives addressed to a tool.
func normalizeComment(comments ...string) string {
	lines := make([]string, 0)
	for _, comment := range comments {
		for line := range strings.SplitSeq(comment, "\n") {
			line = strings.TrimSpace(line)
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if strings.HasPrefix(line, "@") || toolDirective(line) {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "--"))
			if line != "" {
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// toolDirective reports whether one comment line addresses a tool.
func toolDirective(line string) bool {
	for _, directive := range toolDirectives {
		if strings.HasPrefix(line, directive) {
			return true
		}
	}
	return false
}

// commentSource keeps the original values.yaml lines. The YAML decoder removes
// the indentation of a comment, and indentation is what separates a comment
// that documents the following key from one that belongs to the value above it.
type commentSource struct {
	lines []string
}

// newCommentSource splits one values.yaml file into lines.
func newCommentSource(data []byte) *commentSource {
	return &commentSource{lines: strings.Split(string(data), "\n")}
}

// splitHeadComment separates the head comment of a node into the lines that
// document the node and the lines that document the value above it.
//
// A YAML decoder attaches every comment above a node to that node, so a note
// written inside an empty map lands on the next key instead:
//
//	labels:
//	  # Used to add custom labels.
//	annotations:
//
// Indentation separates the two cases, and the decoder removes it, so the
// original file supplies it. A line indented more deeply than the node belongs
// to the previous value.
func (s *commentSource) splitHeadComment(node *yaml.Node) (own, indented string) {
	if node == nil || node.HeadComment == "" {
		return "", ""
	}
	lines := strings.Split(node.HeadComment, "\n")
	if s == nil {
		return node.HeadComment, ""
	}
	columns := s.commentColumns(node.Line, lines)
	if columns == nil {
		return node.HeadComment, ""
	}

	ownLines := make([]string, 0, len(lines))
	indentedLines := make([]string, 0, len(lines))
	for index, line := range lines {
		if columns[index] > node.Column {
			indentedLines = append(indentedLines, line)
			continue
		}
		ownLines = append(ownLines, line)
	}
	return strings.Join(ownLines, "\n"), strings.Join(indentedLines, "\n")
}

// commentColumns reports the original column of each head-comment line.
//
// A decoder splits the comment lines above a node into groups, so a head
// comment is a subsequence of those lines rather than the last few of them.
// Matching the text of each line identifies the group that the decoder kept.
// An unmatched line returns nil, which keeps an unexpected layout on the path
// that changes nothing.
func (s *commentSource) commentColumns(line int, head []string) []int {
	raw := s.commentLinesAbove(line)
	columns := make([]int, len(head))
	position := len(raw) - 1
	for index := len(head) - 1; index >= 0; index-- {
		want := strings.TrimSpace(head[index])
		for position >= 0 && strings.TrimSpace(raw[position]) != want {
			position--
		}
		if position < 0 {
			return nil
		}
		text := raw[position]
		columns[index] = len(text) - len(strings.TrimLeft(text, " \t")) + 1
		position--
	}
	return columns
}

// commentLinesAbove returns the comment lines directly above one file line, in
// file order. Blank lines separate comment groups but do not end the run.
func (s *commentSource) commentLinesAbove(line int) []string {
	above := make([]string, 0, 16)
	for index := line - 2; index >= 0; index-- {
		if index >= len(s.lines) {
			continue
		}
		text := s.lines[index]
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			break
		}
		above = append(above, text)
	}
	slices.Reverse(above)
	return above
}

// appendDescription adds a comment to the description already recorded for one
// value path. It keeps the existing text first, because that text precedes the
// added comment in the file.
func appendDescription(descriptions map[string]string, pointer, comment string) {
	if pointer == "" {
		return
	}
	addition := normalizeComment(comment)
	if addition == "" {
		return
	}
	if existing := descriptions[pointer]; existing != "" {
		descriptions[pointer] = existing + "\n" + addition
		return
	}
	descriptions[pointer] = addition
}
