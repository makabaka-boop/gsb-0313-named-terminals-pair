package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
)

type AnalyzeRequest struct {
	Start       string   `json:"start"`
	Productions []string `json:"productions"`
	Tokens      []string `json:"tokens"`
	RequestID   string   `json:"requestId,omitempty"`
}

type SetInfo struct {
	Terminals []string `json:"terminals"`
	Nullable  bool     `json:"nullable"`
}

type TableCell struct {
	Nonterminal string `json:"nonterminal"`
	Terminal    string `json:"terminal"`
	RuleIDs     []int  `json:"ruleIds"`
}

type Conflict struct {
	Nonterminal string   `json:"nonterminal"`
	Terminal    string   `json:"terminal"`
	RuleIDs     []int    `json:"ruleIds"`
	Candidates  []string `json:"candidates"`
}

type ParseStep struct {
	Step        int    `json:"step"`
	Action      string `json:"action"`
	ActionLabel string `json:"actionLabel"`
	Stack       string `json:"stack"`
	Remaining   string `json:"remaining"`
	UsedRuleID  *int   `json:"usedRuleId,omitempty"`
	Rule        string `json:"rule,omitempty"`
	Message     string `json:"message,omitempty"`
}

type AnalyzeResponse struct {
	RequestID    string              `json:"requestId"`
	Start        string              `json:"start"`
	Nonterminals []string            `json:"nonterminals"`
	Terminals    []string            `json:"terminals"`
	Productions  []string            `json:"productions"`
	Tokens       []string            `json:"tokens"`
	First        map[string]SetInfo  `json:"first"`
	Follow       map[string][]string `json:"follow"`
	Table        []TableCell         `json:"table"`
	Conflict     *Conflict           `json:"conflict,omitempty"`
	Accepted     bool                `json:"accepted"`
	Steps        []ParseStep         `json:"steps,omitempty"`
	Message      string              `json:"message,omitempty"`
}

type grammarRule struct {
	lhs  string
	rhs  []string
	text string
}

type validationError struct {
	message string
}

func (e *validationError) Error() string { return e.message }

func Analyze(req AnalyzeRequest) (AnalyzeResponse, error) {
	start := strings.TrimSpace(req.Start)
	productions := make([]string, 0, len(req.Productions))
	for _, raw := range req.Productions {
		line := strings.TrimSpace(raw)
		if line != "" {
			productions = append(productions, line)
		}
	}
	tokens := make([]string, 0, len(req.Tokens))
	for _, raw := range req.Tokens {
		token := strings.TrimSpace(raw)
		if token != "" {
			tokens = append(tokens, token)
		}
	}

	resp := AnalyzeResponse{
		RequestID:   req.RequestID,
		Start:       start,
		Productions: productions,
		Tokens:      tokens,
		First:       map[string]SetInfo{},
		Follow:      map[string][]string{},
		Table:       []TableCell{},
		Accepted:    false,
	}

	if len(tokens) > 40 {
		return resp, &validationError{"词序列最多包含 40 项"}
	}
	if len(start) != 1 || !isNonterminal(start) {
		return resp, &validationError{"开始符必须是一个大写非终结符"}
	}
	if len(productions) == 0 {
		return resp, &validationError{"至少需要一条产生式"}
	}
	if len(productions) > 30 {
		return resp, &validationError{"产生式最多 30 条"}
	}

	rules := make([]grammarRule, 0, len(productions))
	ntSet := map[string]bool{start: true}
	terminalSet := map[string]bool{}
	seenRule := map[string]bool{}
	lhsHasRule := map[string]bool{}

	for _, production := range productions {
		parts := strings.SplitN(production, "->", 2)
		if len(parts) != 2 {
			return resp, &validationError{"产生式必须形如 A->α：" + production}
		}
		lhsText := strings.TrimSpace(parts[0])
		rhsText := strings.TrimSpace(parts[1])
		if len(lhsText) != 1 || !isNonterminal(lhsText) {
			return resp, &validationError{"产生式左部必须是一个大写非终结符：" + production}
		}
		rhsSymbols, err := tokenizeRHS(rhsText)
		if err != nil {
			return resp, &validationError{"产生式右部符号格式错误：" + production + "（" + err.Error() + "）"}
		}
		if seenRule[production] {
			return resp, &validationError{"重复产生式：" + production}
		}
		seenRule[production] = true

		rules = append(rules, grammarRule{lhs: lhsText, rhs: rhsSymbols, text: production})
		ntSet[lhsText] = true
		lhsHasRule[lhsText] = true
		for _, symbol := range rhsSymbols {
			if isNonterminal(symbol) {
				ntSet[symbol] = true
			} else {
				terminalSet[symbol] = true
			}
		}
	}

	if len(ntSet) > 15 {
		return resp, &validationError{"大写非终结符最多 15 个"}
	}
	if !lhsHasRule[start] {
		return resp, &validationError{"开始符必须至少出现在一条产生式左部"}
	}
	for _, rule := range rules {
		for _, symbol := range rule.rhs {
			if isNonterminal(symbol) && !lhsHasRule[symbol] {
				return resp, &validationError{"非终结符 " + symbol + " 缺少产生式"}
			}
		}
	}
	for _, token := range tokens {
		if token == "$" {
			return resp, &validationError{"$ 是保留的结束符，不能作为输入词"}
		}
	}

	nonterminals := sortedStringSet(ntSet)
	terminals := sortedStringSet(terminalSet)
	terminals = append(terminals, "$")
	resp.Nonterminals = nonterminals
	resp.Terminals = terminals

	nullable := map[string]bool{}
	rhsFirst := make([][]string, len(rules))
	rhsNullable := make([]bool, len(rules))

	// 定点迭代：先单独计算可空性，避免把 FOLLOW 的 $ 或 ε 混入 FIRST。
	for changed := true; changed; {
		changed = false
		for _, rule := range rules {
			if nullable[rule.lhs] {
				continue
			}
			canBeEmpty := true
			for _, symbol := range rule.rhs {
				if isTerminal(symbol) || !nullable[symbol] {
					canBeEmpty = false
					break
				}
			}
			if canBeEmpty {
				nullable[rule.lhs] = true
				changed = true
			}
		}
	}

	firstSets := map[string]map[string]bool{}
	for _, nt := range nonterminals {
		firstSets[nt] = map[string]bool{}
	}
	for changed := true; changed; {
		changed = false
		for ruleIndex, rule := range rules {
			first := firstSets[rule.lhs]
			rhsTerminals := []string{}
			allNullable := len(rule.rhs) == 0
			for _, symbol := range rule.rhs {
				if isTerminal(symbol) {
					rhsTerminals = append(rhsTerminals, symbol)
					if !first[symbol] {
						first[symbol] = true
						changed = true
					}
					allNullable = false
					break
				}
				for terminal := range firstSets[symbol] {
					rhsTerminals = append(rhsTerminals, terminal)
					if !first[terminal] {
						first[terminal] = true
						changed = true
					}
				}
				if !nullable[symbol] {
					allNullable = false
					break
				}
			}
			rhsFirst[ruleIndex] = rhsTerminals
			rhsNullable[ruleIndex] = allNullable
		}
	}

	followSets := map[string]map[string]bool{}
	for _, nt := range nonterminals {
		followSets[nt] = map[string]bool{}
	}
	followSets[start]["$"] = true

	for changed := true; changed; {
		changed = false
		addFollow := func(from, terminal string) {
			if terminal != "$" && !isTerminal(terminal) {
				return
			}
			if !followSets[from][terminal] {
				followSets[from][terminal] = true
				changed = true
			}
		}

		for _, rule := range rules {
			rhs := rule.rhs
			for i, symbol := range rhs {
				if !isNonterminal(symbol) {
					continue
				}
				betaFirst := []string{}
				betaNullable := true
				for j := i + 1; j < len(rhs); j++ {
					next := rhs[j]
					if isTerminal(next) {
						betaFirst = append(betaFirst, next)
						betaNullable = false
						break
					}
					for terminal := range firstSets[next] {
						betaFirst = append(betaFirst, terminal)
					}
					if !nullable[next] {
						betaNullable = false
						break
					}
				}
				for _, terminal := range betaFirst {
					addFollow(symbol, terminal)
				}
				if betaNullable {
					for terminal := range followSets[rule.lhs] {
						addFollow(symbol, terminal)
					}
				}
			}
		}
	}

	for _, nt := range resp.Nonterminals {
		resp.First[nt] = SetInfo{
			Terminals: sortedStringSet(firstSets[nt]),
			Nullable:  nullable[nt],
		}
		resp.Follow[nt] = sortedStringSet(followSets[nt])
	}

	tableMap := map[string]*TableCell{}
	for ruleIndex, rule := range rules {
		for _, terminal := range rhsFirst[ruleIndex] {
			addTableCell(tableMap, rule.lhs, terminal, ruleIndex)
		}
		if rhsNullable[ruleIndex] {
			for terminal := range followSets[rule.lhs] {
				addTableCell(tableMap, rule.lhs, terminal, ruleIndex)
			}
		}
	}

	table := make([]TableCell, 0, len(tableMap))
	var conflict *Conflict
	for _, cell := range tableMap {
		sort.Ints(cell.RuleIDs)
		table = append(table, *cell)
		if len(cell.RuleIDs) > 1 {
			if conflict == nil || symbolPairLess(cell.Nonterminal, cell.Terminal, conflict.Nonterminal, conflict.Terminal) {
				candidates := make([]string, 0, len(cell.RuleIDs))
				for _, id := range cell.RuleIDs {
					candidates = append(candidates, rules[id].text)
				}
				conflict = &Conflict{
					Nonterminal: cell.Nonterminal,
					Terminal:    cell.Terminal,
					RuleIDs:     append([]int(nil), cell.RuleIDs...),
					Candidates:  candidates,
				}
			}
		}
	}
	sort.Slice(table, func(i, j int) bool {
		return symbolPairLess(table[i].Nonterminal, table[i].Terminal, table[j].Nonterminal, table[j].Terminal)
	})
	resp.Table = table
	if conflict != nil {
		resp.Conflict = conflict
		resp.Message = "LL(1) 表存在冲突，未执行预测分析"
		return resp, nil
	}

	lookup := map[string]int{}
	for _, cell := range table {
		if len(cell.RuleIDs) == 1 {
			lookup[cell.Nonterminal+"\x00"+cell.Terminal] = cell.RuleIDs[0]
		}
	}
	resp.Steps = runPredictiveParser(start, rules, tokens, lookup, terminalSet)
	resp.Accepted = len(resp.Steps) > 0 && resp.Steps[len(resp.Steps)-1].Action == "accept"
	if !resp.Accepted {
		resp.Message = resp.Steps[len(resp.Steps)-1].Message
	}
	return resp, nil
}

func runPredictiveParser(start string, rules []grammarRule, tokens []string, lookup map[string]int, grammarTerminals map[string]bool) []ParseStep {
	// 栈顶在末尾，元素是文法符号：非终结符、单字母或具名终结符。
	stack := []string{"$", start}
	remainingTokens := append([]string(nil), tokens...)
	steps := []ParseStep{}
	snapshot := func(action, label, message string, ruleID *int, ruleText string) ParseStep {
		stackCopy := append([]string(nil), stack...)
		for i, j := 0, len(stackCopy)-1; i < j; i, j = i+1, j-1 {
			stackCopy[i], stackCopy[j] = stackCopy[j], stackCopy[i]
		}
		return ParseStep{
			Step:        len(steps) + 1,
			Action:      action,
			ActionLabel: label,
			Stack:       joinTokenDisplay(stackCopy),
			Remaining:   joinTokenDisplay(append(append([]string(nil), remainingTokens...), "$")),
			UsedRuleID:  ruleID,
			Rule:        ruleText,
			Message:     message,
		}
	}

	for {
		top := stack[len(stack)-1]
		currentToken := "$"
		if len(remainingTokens) > 0 {
			currentToken = remainingTokens[0]
		}

		if top == "$" && currentToken == "$" {
			steps = append(steps, snapshot("accept", "接受", "", nil, ""))
			return steps
		}

		if currentToken != "$" {
			if !grammarTerminals[currentToken] {
				message := "非法词：" + currentToken
				steps = append(steps, snapshot("error", "错误", message, nil, ""))
				return steps
			}
		}

		if isTerminal(top) || top == "$" {
			if top != currentToken {
				expected := displaySymbol(top)
				got := displaySymbol(currentToken)
				message := "匹配失败：期望 " + expected + "，实际遇到 " + got
				steps = append(steps, snapshot("error", "错误", message, nil, ""))
				return steps
			}
			stack = stack[:len(stack)-1]
			remainingTokens = remainingTokens[1:]
			steps = append(steps, snapshot("match", "匹配 "+displaySymbol(top), "", nil, ""))
			continue
		}

		ruleID, ok := lookup[top+"\x00"+currentToken]
		if !ok {
			message := "分析表 M[" + top + ", " + displaySymbol(currentToken) + "] 为空，无法选择产生式"
			steps = append(steps, snapshot("error", "错误", message, nil, ""))
			return steps
		}

		stack = stack[:len(stack)-1]
		rhs := rules[ruleID].rhs
		for i := len(rhs) - 1; i >= 0; i-- {
			stack = append(stack, rhs[i])
		}
		id := ruleID
		rhsLabel := formatRHS(rhs)
		steps = append(steps, snapshot("expand", "展开", "", &id, rules[ruleID].lhs+" -> "+rhsLabel))
	}
}

// tokenizeRHS 把产生式右部切成原子文法符号：
// 大写字母是非终结符，单个小写字母是单字母终结符，{name} 是具名终结符（一个完整词）。
func tokenizeRHS(value string) ([]string, error) {
	symbols := []string{}
	for i := 0; i < len(value); {
		switch {
		case value[i] == '{':
			end := strings.IndexByte(value[i+1:], '}')
			if end < 0 {
				return nil, errors.New("花括号 { 缺少配对的 }")
			}
			name := value[i+1 : i+1+end]
			if len(name) < 2 || len(name) > 12 {
				return nil, errors.New("具名终结符名称长度必须为 2～12 位小写字母")
			}
			for j := 0; j < len(name); j++ {
				if !isTerminalByte(name[j]) {
					return nil, errors.New("具名终结符只能使用小写字母 a-z：{" + name + "}")
				}
			}
			symbols = append(symbols, name)
			i += end + 2
		case value[i] == '}':
			return nil, errors.New("花括号 } 缺少配对的 {")
		case isNonterminalByte(value[i]):
			symbols = append(symbols, string(value[i]))
			i++
		case isTerminalByte(value[i]):
			symbols = append(symbols, string(value[i]))
			i++
		default:
			return nil, errors.New("右部只能包含大写非终结符、小写字母或 {名称}")
		}
	}
	return symbols, nil
}

func addTableCell(tableMap map[string]*TableCell, lhs, terminal string, ruleID int) {
	key := lhs + "\x00" + terminal
	cell := tableMap[key]
	if cell == nil {
		cell = &TableCell{Nonterminal: lhs, Terminal: terminal}
		tableMap[key] = cell
	}
	for _, existing := range cell.RuleIDs {
		if existing == ruleID {
			return
		}
	}
	cell.RuleIDs = append(cell.RuleIDs, ruleID)
}

func isNonterminal(value string) bool {
	return len(value) == 1 && value[0] >= 'A' && value[0] <= 'Z'
}

func isTerminal(value string) bool {
	if len(value) == 0 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 'a' || value[i] > 'z' {
			return false
		}
	}
	return true
}

func isNonterminalByte(value byte) bool {
	return value >= 'A' && value <= 'Z'
}

func isTerminalByte(value byte) bool {
	return value >= 'a' && value <= 'z'
}

// formatRHS 把右部符号序列还原成产生式文本，具名终结符仍带花括号。
func formatRHS(symbols []string) string {
	if len(symbols) == 0 {
		return "ε"
	}
	var builder strings.Builder
	for _, symbol := range symbols {
		if isNonterminal(symbol) {
			builder.WriteString(symbol)
		} else {
			builder.WriteString("{" + symbol + "}")
		}
	}
	return builder.String()
}

// displaySymbol 用于面向用户的消息：具名终结符显示为 {name}，$ 显示为结束符 $。
func displaySymbol(value string) string {
	if value == "$" {
		return "结束符 $"
	}
	if isTerminal(value) && len(value) > 1 {
		return "{" + value + "}"
	}
	return value
}

// joinTokenDisplay 渲染栈回放中的符号序列：相邻的单字母符号与 $ 紧凑拼接，
// 具名终结符加花括号并与相邻符号保留空格，确保 {id} 不被读成 i、d。
func joinTokenDisplay(symbols []string) string {
	type piece struct {
		text string
		wide bool
	}
	parts := make([]piece, 0, len(symbols))
	for _, symbol := range symbols {
		if isTerminal(symbol) && len(symbol) > 1 {
			parts = append(parts, piece{"{" + symbol + "}", true})
		} else {
			parts = append(parts, piece{symbol, false})
		}
	}
	var builder strings.Builder
	for index, part := range parts {
		if index > 0 && (part.wide || parts[index-1].wide) {
			builder.WriteString(" ")
		}
		builder.WriteString(part.text)
	}
	return builder.String()
}

func sortedStringSet(values map[string]bool) []string {
	list := make([]string, 0, len(values))
	for value := range values {
		list = append(list, value)
	}
	sort.Strings(list)
	return list
}

func symbolPairLess(aNonterminal, aTerminal, bNonterminal, bTerminal string) bool {
	if aNonterminal != bNonterminal {
		return aNonterminal < bNonterminal
	}
	return aTerminal < bTerminal
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
