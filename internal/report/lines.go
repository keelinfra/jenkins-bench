package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/keelinfra/jenkins-bench/internal/checks"
	"github.com/keelinfra/jenkins-bench/internal/console"
	"github.com/keelinfra/jenkins-bench/internal/engine"
)

// The default report body: one finding per record, the shape linters use.
//
// A failure is read in one place — which resource, which control, why, and the
// fix — instead of being split across a summary matrix, an aggregate table and
// a remediation section three screens apart. The label column is the record
// delimiter: a finding starts with its severity two columns in, and everything
// that belongs to it is indented past that, so the eye and grep both parse it.

// writeFindingLines renders the findings: FAIL records, PASS records (with
// ShowPassed), aggregated MANUAL groups, aggregated N/A groups (with
// ShowPassed), then the one-sentence unread summary. Unread findings are not
// listed per record — they are one cause, explained once in the scan warnings.
func writeFindingLines(w io.Writer, rep *engine.Report, p painter, width int, opts Options) {
	var fails, passes, accepted []engine.Finding
	for _, f := range rep.Findings {
		switch {
		case f.Waiver != nil:
			accepted = append(accepted, f)
		case f.Status == engine.StatusFail:
			fails = append(fails, f)
		case f.Status == engine.StatusPass && opts.ShowPassed:
			passes = append(passes, f)
		}
	}
	manuals := groupByControlDetails(rep.Findings, func(f engine.Finding) bool {
		return f.Status == engine.StatusManual && !unread(f) && f.Waiver == nil
	})
	var nas []findingGroup
	if opts.ShowPassed {
		nas = groupByControlDetails(rep.Findings, func(f engine.Finding) bool {
			return f.Status == engine.StatusNA
		})
	}
	unreadSentence, unreadIDs := unreadSummary(rep.Findings)

	if len(fails) == 0 && len(passes) == 0 && len(manuals) == 0 && len(nas) == 0 && len(accepted) == 0 && unreadSentence == "" {
		blank(w)
		line(w, "%s", p.paint(ansiGreen, "No failed or manual-review controls."))
		return
	}

	if len(fails) > 0 {
		meta := console.Pluralize(len(fails), "finding")
		if n := failedControls(fails); n != len(fails) {
			meta += " in " + console.Pluralize(n, "control")
		}
		heading(w, p, width, ansiRed, "Failed", meta)
		for _, f := range controllerFirst(fails) {
			blank(w)
			writeFindingLine(w, p, width, f, opts)
		}
	}
	if len(passes) > 0 {
		heading(w, p, width, ansiGreen, "Passed", console.Pluralize(len(passes), "finding"))
		for _, f := range controllerFirst(passes) {
			blank(w)
			writeFindingLine(w, p, width, f, opts)
		}
	}
	if len(manuals) > 0 {
		heading(w, p, width, ansiYellow, "Manual review", console.Pluralize(len(manuals), "control"))
		writeGroupLines(w, p, width, statusManual, manuals, opts)
	}
	if len(nas) > 0 {
		heading(w, p, width, "", "Not applicable", console.Pluralize(len(nas), "control"))
		writeGroupLines(w, p, width, statusNA, nas, opts)
	}
	// Accepted findings last among the findings: still listed, so an exception
	// never makes a problem invisible, but below everything that fails the run.
	if len(accepted) > 0 {
		heading(w, p, width, "", "Accepted by exceptions", fmt.Sprintf("%d", len(accepted)))
		for _, f := range controllerFirst(accepted) {
			blank(w)
			record(w, p, width, string(f.Status), func(s string) string { return p.paint(ansiDim, s) }, f.CheckID, f.Resource)
			recordText(w, p, width, "", acceptedNote(f.Waiver))
		}
	}

	if unreadSentence != "" {
		if len(rep.Metadata.Warnings) > 0 {
			unreadSentence += "; see Scan warnings below."
		} else {
			// Nothing in the warnings explains it — the cause is the control's
			// own, such as an update site never fetched — so the sentence has to
			// say where the explanation is or it is a dead end.
			unreadSentence += "; rerun with --details=" + strings.Join(unreadIDs, ",") + " for why."
		}
		heading(w, p, width, ansiYellow, "Unread", console.Pluralize(len(unreadIDs), "control"))
		blank(w)
		indented(w, p, width, ansiYellow, unreadSentence)
	}
}

// writeFindingLine renders one record:
//
//	HIGH    CIS-2.1.2  platform/nightly-deploy
//	        <details>
//	        fix  <one-line remediation>
//	        ·    <evidence>
//
// The label column is the severity for a failure and the state for anything
// else, so the column says, top to bottom, how much each record matters.
func writeFindingLine(w io.Writer, p painter, width int, f engine.Finding, opts Options) {
	sev := strings.ToUpper(f.Severity)
	label, labelColour := sev, severityColour(p)
	if f.Status == engine.StatusPass {
		label, labelColour = statusPass, statusColour(p)
	}
	record(w, p, width, label, labelColour, f.CheckID, f.Resource)
	recordText(w, p, width, "", f.Details)

	if f.Status != engine.StatusFail {
		return
	}
	if !opts.NoRemediations {
		recordKey(w, p, width, "fix", ansiBold, fixLine(f), "")
	}
	for _, e := range f.Evidence {
		recordKey(w, p, width, "·", ansiDim, e, ansiDim)
	}
}

// findingGroup is one aggregated line: a control, the shared reason, and the
// resources it covers.
type findingGroup struct {
	rep       engine.Finding // representative: severity, fix, IDs
	details   string
	resources []string
	instance  bool
	kinds     map[string]bool
}

// groupByControlDetails folds findings into one group per (CheckID, Details).
//
// The pair, not the CheckID alone: two resources can be MANUAL for different
// reasons, and collapsing them under whichever reason came first would be a
// small lie. In practice the Details of a MANUAL control are constant across
// resources, so a control almost always folds to a single line.
func groupByControlDetails(findings []engine.Finding, keep func(engine.Finding) bool) []findingGroup {
	index := map[string]int{}
	var out []findingGroup
	for _, f := range findings {
		if !keep(f) {
			continue
		}
		key := f.CheckID + "\x00" + f.Details
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, findingGroup{rep: f, details: f.Details, kinds: map[string]bool{}})
		}
		out[i].resources = append(out[i].resources, f.Resource)
		out[i].kinds[f.ResourceType] = true
		if f.Resource == engine.InstanceResourceName {
			out[i].instance = true
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return lessGroup(out[a], out[b]) })
	return out
}

// lessGroup orders the aggregated lines: most severe first, then benchmark
// order, then the reason that covers the most resources, then by text.
func lessGroup(x, y findingGroup) bool {
	if wa, wb := checks.Weight(x.rep.Severity), checks.Weight(y.rep.Severity); wa != wb {
		return wa > wb
	}
	if x.rep.CISID != y.rep.CISID {
		return checks.LessCISID(x.rep.CISID, y.rep.CISID)
	}
	if len(x.resources) != len(y.resources) {
		return len(x.resources) > len(y.resources)
	}
	return x.details < y.details
}

// countLabel names what a group spans: the instance by name, anything else by
// count and resource kind.
func (g findingGroup) countLabel() string {
	if g.instance && len(g.resources) == 1 {
		return engine.InstanceResourceName
	}
	noun := "resource"
	if len(g.kinds) == 1 {
		for k := range g.kinds {
			if k != "" {
				noun = strings.ToLower(k)
			}
		}
	}
	return console.Pluralize(len(g.resources), noun)
}

// writeGroupLines renders aggregated records:
//
//	MANUAL  CIS-2.1.1  5 jobs
//	        <details>
//	        fix  <one-line remediation>
//
// One question needing a person is one question however many resources it
// spans; the per-resource split is a --details away.
func writeGroupLines(w io.Writer, p painter, width int, status string, groups []findingGroup, opts Options) {
	for _, g := range groups {
		blank(w)
		record(w, p, width, status, statusColour(p), g.rep.CheckID, g.countLabel())
		recordText(w, p, width, "", g.details)

		if status == statusManual && !opts.NoRemediations {
			recordKey(w, p, width, "fix", ansiBold, fixLine(g.rep), "")
		}
	}
}

// controllerFirst orders findings so the instance's records lead within each
// control; everything else keeps the engine's order (severity, benchmark
// number, resource).
func controllerFirst(findings []engine.Finding) []engine.Finding {
	out := make([]engine.Finding, len(findings))
	copy(out, findings)
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.CheckID == y.CheckID {
			return x.Resource == engine.InstanceResourceName && y.Resource != engine.InstanceResourceName
		}
		return false
	})
	return out
}

// unreadSummary folds every unread finding into one clause, or "" when
// nothing went unread, and names the controls so the reader knows which ones
// to look up. The cause — a 401, a missing permission — is usually already a
// scan warning, so the caller points there instead of repeating it per
// control per resource; when no warning explains it, the caller points at
// --details for these controls instead.
func unreadSummary(findings []engine.Finding) (string, []string) {
	var ids []string
	controls := map[string]bool{}
	resources := map[string]bool{}
	var one string
	for _, f := range findings {
		if !unread(f) {
			continue
		}
		if !controls[f.CheckID] {
			ids = append(ids, f.CheckID)
		}
		controls[f.CheckID] = true
		resources[f.Resource] = true
		one = f.Resource
	}
	if len(controls) == 0 {
		return "", nil
	}
	where := "on " + one
	if len(resources) > 1 {
		where = fmt.Sprintf("across %s", console.Pluralize(len(resources), "resource"))
	}
	named := ids
	more := ""
	if len(named) > maxNamedUnread {
		more = fmt.Sprintf(" and %d more", len(named)-maxNamedUnread)
		named = named[:maxNamedUnread]
	}
	return fmt.Sprintf("%s could not be read (Unread) %s: %s%s",
		console.Pluralize(len(controls), "control"), where, strings.Join(named, ", "), more), ids
}

// maxNamedUnread caps how many unread controls the summary names. A token that
// can read almost nothing leaves most of the bundle unread, and the warnings
// say why; the list is for finding the few, not for restating the bundle.
const maxNamedUnread = 6

// writeHint closes the report body with where the rest lives. It is the
// discoverability mechanism for --details, so it always prints in the default
// layout.
func writeHint(w io.Writer, p painter, width int) {
	blank(w)
	text := "Details: rerun with --details for per-resource findings and full remediation steps, " +
		"or --details=<resource|control>[,...] to filter; -o json for the full report."
	indented(w, p, width, ansiDim, text)
}

// detailFilter is the parsed form of --details: which controls and which
// resources the per-resource sections should cover. Empty means everything.
type detailFilter struct {
	controls  map[string]bool // canonical CheckIDs, matched exactly
	resources []string        // lowercased substrings of resource names
}

func (f detailFilter) empty() bool { return len(f.controls) == 0 && len(f.resources) == 0 }

// parseDetailFilters classifies each --details value against the report.
//
// A value is a control filter when, case-insensitively and with an optional
// CIS- prefix supplied, it exactly equals a CheckID in the report — exact,
// because a substring match on dotted numbers would make "1.1" silently mean
// nine controls. Anything else is a resource filter, matched as a
// case-insensitive substring of the resource name. A value matching neither is
// an error rather than a warning: a filter that matched nothing would render a
// report indistinguishable from a clean one.
//
// "all" is the sentinel a bare --details produces and matches everything.
func parseDetailFilters(values []string, findings []engine.Finding) (detailFilter, error) {
	ids := map[string]string{}
	for _, f := range findings {
		ids[strings.ToLower(f.CheckID)] = f.CheckID
	}

	out := detailFilter{controls: map[string]bool{}}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || strings.EqualFold(v, "all") {
			continue
		}
		lower := strings.ToLower(v)
		if id, ok := ids[lower]; ok {
			out.controls[id] = true
			continue
		}
		if id, ok := ids["cis-"+lower]; ok {
			out.controls[id] = true
			continue
		}
		matched := false
		for _, f := range findings {
			if strings.Contains(strings.ToLower(f.Resource), lower) {
				matched = true
				break
			}
		}
		if !matched {
			return detailFilter{}, fmt.Errorf(
				"--details %q matched no control ID and no resource in this report; resource names appear in the scan output, controls in list-checks", v)
		}
		out.resources = append(out.resources, lower)
	}
	return out, nil
}

// filterFindings keeps the findings the filter selects: resources matching any
// resource value (all, when none were given) AND controls matching any control
// value (likewise).
func filterFindings(findings []engine.Finding, filter detailFilter) []engine.Finding {
	if filter.empty() {
		return findings
	}
	var out []engine.Finding
	for _, f := range findings {
		if len(filter.controls) > 0 && !filter.controls[f.CheckID] {
			continue
		}
		if len(filter.resources) > 0 {
			lower := strings.ToLower(f.Resource)
			hit := false
			for _, sub := range filter.resources {
				if strings.Contains(lower, sub) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}
