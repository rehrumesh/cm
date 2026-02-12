package logview

import "testing"

func TestSelectionLifecycle(t *testing.T) {
	s := NewSelection()

	s.Start(13, 9, 2, 10, 5) // content origin is (11,7) => line=2 col=2
	if !s.Selecting {
		t.Fatalf("expected selecting=true after start")
	}
	if s.Selected {
		t.Fatalf("expected selected=false after start")
	}
	if s.PaneIdx != 2 {
		t.Fatalf("expected pane idx 2, got %d", s.PaneIdx)
	}

	s.Update(18, 12) // => line=5 col=7
	startLine, startCol, endLine, endCol := s.GetNormalizedRange()
	if startLine != 2 || startCol != 2 || endLine != 5 || endCol != 7 {
		t.Fatalf("unexpected range: (%d,%d) to (%d,%d)", startLine, startCol, endLine, endCol)
	}

	if !s.Finalize() {
		t.Fatalf("expected finalize to report selected text")
	}
	if s.Selecting {
		t.Fatalf("expected selecting=false after finalize")
	}
	if !s.Selected {
		t.Fatalf("expected selected=true after finalize")
	}
}

func TestSelectionNormalizeReverseDrag(t *testing.T) {
	s := NewSelection()
	s.Start(20, 12, 0, 10, 5) // line=5 col=9
	s.Update(12, 8)           // line=1 col=1

	startLine, startCol, endLine, endCol := s.GetNormalizedRange()
	if startLine != 1 || startCol != 1 || endLine != 5 || endCol != 9 {
		t.Fatalf("unexpected normalized range: (%d,%d) to (%d,%d)", startLine, startCol, endLine, endCol)
	}
}

func TestSelectionFinalizeEmptyRange(t *testing.T) {
	s := NewSelection()
	s.Start(15, 10, 1, 10, 5)
	if s.Finalize() {
		t.Fatalf("expected finalize to report no selected text for click-only selection")
	}
	if s.Selected {
		t.Fatalf("expected selected=false after empty finalize")
	}
}

func TestSelectionHasSelectedText(t *testing.T) {
	s := NewSelection()
	s.Start(13, 9, 0, 10, 5)
	s.Update(15, 9) // same line, wider col range
	if !s.HasSelectedText() {
		t.Fatalf("expected selected text on same-line range")
	}

	s2 := NewSelection()
	s2.Start(13, 9, 0, 10, 5)
	s2.Update(13, 11) // different line, same col
	if !s2.HasSelectedText() {
		t.Fatalf("expected selected text on multi-line range")
	}

	s3 := NewSelection()
	s3.Start(13, 9, 0, 10, 5)
	if s3.HasSelectedText() {
		t.Fatalf("expected no selected text on zero-width selection")
	}
}
