package main

import (
	"reflect"
	"strings"
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

func TestNamedTerminalIsOneAtomAndAccepts(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->{id}R", "R->{plus}{id}R", "R->"},
		Tokens:      []string{"id", "plus", "id"},
		RequestID:   "named-accept",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", resp.Conflict)
	}
	if got := resp.Terminals; !reflect.DeepEqual(got, []string{"id", "plus", "$"}) {
		t.Fatalf("terminals = %v, want [id plus $], no single-letter i/d columns", got)
	}
	if got := resp.First["S"].Terminals; !reflect.DeepEqual(got, []string{"id"}) {
		t.Fatalf("FIRST(S) = %v, want [id] as a single terminal", got)
	}
	if !resp.Accepted {
		t.Fatalf("named example should accept, steps = %+v", resp.Steps)
	}
	last := resp.Steps[len(resp.Steps)-1]
	if last.Action != "accept" || last.Stack != "$" || last.Remaining != "$" {
		t.Fatalf("last step = %+v, want accept at $/$", last)
	}
}

func TestNamedAndLegacyTerminalStayDistinct(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->{id}R", "S->iR", "R->"},
		Tokens:      []string{"id"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Conflict != nil {
		t.Fatalf("{id} and legacy i are distinct terminals, no conflict: %+v", resp.Conflict)
	}
	if got := resp.First["S"].Terminals; !reflect.DeepEqual(got, []string{"i", "id"}) {
		t.Fatalf("FIRST(S) = %v, want distinct [i id]", got)
	}
	if !resp.Accepted {
		t.Fatalf("token id must parse through the {id} rule, steps = %+v", resp.Steps)
	}

	// 同一文法下输入旧式单字母 i，必须走另一条规则，两者不能混用。
	respI, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->{id}R", "S->iR", "R->"},
		Tokens:      []string{"i"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if !respI.Accepted {
		t.Fatalf("token i must parse through the legacy single-letter rule, steps = %+v", respI.Steps)
	}
}

func TestNamedTerminalConflictReportedAsOneColumn(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->A{id}", "A->{id}", "A->"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Conflict == nil {
		t.Fatal("expected an LL(1) conflict at (A,id)")
	}
	if resp.Conflict.Nonterminal != "A" || resp.Conflict.Terminal != "id" {
		t.Fatalf("conflict cell = (%s,%s), want (A,id)", resp.Conflict.Nonterminal, resp.Conflict.Terminal)
	}
	if got := resp.Conflict.Candidates; !reflect.DeepEqual(got, []string{"A->{id}", "A->"}) {
		t.Fatalf("candidates = %v", got)
	}
	if resp.Steps != nil {
		t.Fatalf("conflicting grammar must not fabricate parser steps: %v", resp.Steps)
	}
}

func TestNamedTerminalAroundNullableProduction(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->{id}B{plus}", "B->{num}", "B->"},
		Tokens:      []string{"id", "plus"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if !resp.First["B"].Nullable {
		t.Fatalf("B should be nullable: %+v", resp.First["B"])
	}
	if got := resp.Follow["B"]; !reflect.DeepEqual(got, []string{"plus"}) {
		t.Fatalf("FOLLOW(B) = %v, want [plus] from the named terminal after B", got)
	}
	if !resp.Accepted {
		t.Fatalf("id plus should accept by skipping nullable B, steps = %+v", resp.Steps)
	}

	// 具名终结符在可空符号之前：它必须作为 B 后方序列的首个终结符驱动 FIRST 入表。
	respFirst, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->B{num}", "B->"},
		Tokens:      []string{"num"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if got := respFirst.First["S"].Terminals; !reflect.DeepEqual(got, []string{"num"}) {
		t.Fatalf("FIRST(S) = %v, want [num]", got)
	}
	if !respFirst.Accepted {
		t.Fatalf("num should accept, steps = %+v", respFirst.Steps)
	}
}

func TestNamedTokenFailureShowsActualRemaining(t *testing.T) {
	// num 是文法中的合法词（出现在另一条规则），但此处期望 plus：首个失败停在匹配失败。
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->{id}{plus}", "S->{num}"},
		Tokens:      []string{"id", "num"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Accepted {
		t.Fatal("input should not accept")
	}
	last := resp.Steps[len(resp.Steps)-1]
	if last.Message != "匹配失败：期望 {plus}，实际遇到 {num}" {
		t.Fatalf("message = %q", last.Message)
	}
	if last.Remaining != "{num} $" {
		t.Fatalf("remaining = %q, want {num} $", last.Remaining)
	}
	if last.Stack != "{plus} $" {
		t.Fatalf("stack = %q, want {plus} $", last.Stack)
	}
}

func TestUnknownNamedTokenReportedAtFirstFailure(t *testing.T) {
	resp, err := Analyze(AnalyzeRequest{
		Start:       "S",
		Productions: []string{"S->{id}{plus}"},
		Tokens:      []string{"id", "num"},
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if resp.Accepted {
		t.Fatal("input should not accept")
	}
	last := resp.Steps[len(resp.Steps)-1]
	if last.Message != "非法词：num" {
		t.Fatalf("message = %q, want 非法词：num", last.Message)
	}
	if last.Remaining != "{num} $" {
		t.Fatalf("remaining = %q, want {num} $", last.Remaining)
	}
	if last.Stack != "{plus} $" {
		t.Fatalf("stack = %q, want {plus} $", last.Stack)
	}
}

func TestMalformedNamedProductionsGiveStableError(t *testing.T) {
	cases := []struct {
		production string
		want       string
	}{
		{"S->{id", "花括号"},
		{"S->id}", "花括号"},
		{"S->{}", "长度"},
		{"S->{1a}", "小写字母"},
		{"S->{abcdefghijklm}", "长度"},
		{"S->{i d}", "小写字母"},
	}
	for _, tc := range cases {
		_, err := Analyze(AnalyzeRequest{
			Start:       "S",
			Productions: []string{tc.production},
		})
		if err == nil {
			t.Fatalf("production %q should be rejected", tc.production)
		}
		message := err.Error()
		if !strings.Contains(message, tc.want) {
			t.Fatalf("production %q error = %q, want it to mention %q", tc.production, message, tc.want)
		}
	}
}
