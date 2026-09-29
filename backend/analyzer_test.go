package main

import (
	"reflect"
	"testing"
)

func TestIndirectEpsilonPropagation(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->aB", "S->BC", "B->b", "B->", "C->c", "C->"},
		Tokens:      []string{"a"},
		RequestID:   "epsilon",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", resp.Conflict)
	}
	if !resp.First["S"].Nullable || !resp.First["B"].Nullable || !resp.First["C"].Nullable {
		t.Fatalf("nullable sets = S:%v B:%v C:%v, want all true", resp.First["S"].Nullable, resp.First["B"].Nullable, resp.First["C"].Nullable)
	}
	if got := resp.First["S"].Terminals; !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("FIRST(S) = %v, want [a b c]", got)
	}
	if got := resp.Follow["B"]; !reflect.DeepEqual(got, []string{"$", "c"}) {
		t.Fatalf("FOLLOW(B) = %v, want [$ c]", got)
	}
	if !resp.Accepted {
		t.Fatalf("accepted = false, steps = %+v", resp.Steps)
	}
}

func TestFollowConflict(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->Aa", "A->a", "A->"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Conflict == nil {
		t.Fatal("expected an LL(1) conflict")
	}
	if resp.Conflict.Nonterminal != "A" || resp.Conflict.Terminal != "a" {
		t.Fatalf("smallest conflict = (%s,%s), want (A,a)", resp.Conflict.Nonterminal, resp.Conflict.Terminal)
	}
	wantCandidates := []string{"A->a", "A->"}
	if !reflect.DeepEqual(resp.Conflict.Candidates, wantCandidates) {
		t.Fatalf("candidates = %v, want %v", resp.Conflict.Candidates, wantCandidates)
	}
	if resp.Steps != nil {
		t.Fatalf("conflicting grammar must not fabricate parser steps: %v", resp.Steps)
	}
}

func TestRecursiveGrammarFixedPointAndFailureStep(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "E",
		Productions: []string{"E->TR", "R->pTR", "R->", "T->i", "T->oEc"},
		Tokens:      []string{"i", "p", "o"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", resp.Conflict)
	}
	if got := resp.First["R"].Terminals; !reflect.DeepEqual(got, []string{"p"}) || !resp.First["R"].Nullable {
		t.Fatalf("FIRST(R) = %+v, want p and nullable", resp.First["R"])
	}
	if got := resp.Follow["R"]; !reflect.DeepEqual(got, []string{"$", "c"}) {
		t.Fatalf("FOLLOW(R) = %v, want [$ c]", got)
	}
	if resp.Accepted {
		t.Fatal("input should not be accepted")
	}
	last := resp.Steps[len(resp.Steps)-1]
	if last.Action != "error" || last.Message == "" {
		t.Fatalf("last step = %+v, want the first recorded failure", last)
	}
	if last.Remaining != "$" {
		t.Fatalf("remaining at failure = %q, want $", last.Remaining)
	}
	if last.Stack != "EcR$" {
		t.Fatalf("stack at failure = %q, want EcR$", last.Stack)
	}
}

func TestReturnsIllegalTokenAtFirstFailureStep(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->a"},
		Tokens:      []string{"id"},
	})
	if err != nil {
		t.Fatalf("illegal token should be reported in parser steps, got validation error: %v", err)
	}
	if resp.Accepted || len(resp.Steps) != 1 {
		t.Fatalf("steps = %+v, want one illegal-token failure", resp.Steps)
	}
	if resp.Steps[0].Message != "非法词：id" {
		t.Fatalf("message = %q", resp.Steps[0].Message)
	}
}

func TestRejectsDollarAsReservedInputToken(t *testing.T) {
	_, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->a"},
		Tokens:      []string{"$"},
	})
	if err == nil {
		t.Fatal("expected reserved $ validation error")
	}
}
