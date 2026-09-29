package main

import (
	"encoding/json"
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
	lhs  byte
	rhs  string
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
	if len(start) != 1 || !isNonterminalByte(start[0]) {
		return resp, &validationError{"开始符必须是一个大写非终结符"}
	}
	if len(productions) == 0 {
		return resp, &validationError{"至少需要一条产生式"}
	}
	if len(productions) > 30 {
		return resp, &validationError{"产生式最多 30 条"}
	}

	rules := make([]grammarRule, 0, len(productions))
	ntSet := map[byte]bool{start[0]: true}
	terminalSet := map[byte]bool{}
	seenRule := map[string]bool{}
	lhsHasRule := map[byte]bool{}

	for _, production := range productions {
		parts := strings.SplitN(production, "->", 2)
		if len(parts) != 2 {
			return resp, &validationError{"产生式必须形如 A->α：" + production}
		}
		lhsText := strings.TrimSpace(parts[0])
		rhs := strings.TrimSpace(parts[1])
		if len(lhsText) != 1 || !isNonterminalByte(lhsText[0]) {
			return resp, &validationError{"产生式左部必须是一个大写非终结符：" + production}
		}
		flatRHS, ok := flattenNamedTerminals(rhs)
		if !ok {
			return resp, &validationError{"产生式右部符号格式错误：" + production}
		}
		if seenRule[production] {
			return resp, &validationError{"重复产生式：" + production}
		}
		seenRule[production] = true

		lhs := lhsText[0]
		rules = append(rules, grammarRule{lhs: lhs, rhs: flatRHS, text: production})
		ntSet[lhs] = true
		lhsHasRule[lhs] = true
		for i := 0; i < len(rhs); i++ {
			if isNonterminalByte(rhs[i]) {
				ntSet[rhs[i]] = true
			} else {
				terminalSet[rhs[i]] = true
			}
		}
	}

	if len(ntSet) > 15 {
		return resp, &validationError{"大写非终结符最多 15 个"}
	}
	if !lhsHasRule[start[0]] {
		return resp, &validationError{"开始符必须至少出现在一条产生式左部"}
	}
	for _, rule := range rules {
		for i := 0; i < len(rule.rhs); i++ {
			symbol := rule.rhs[i]
			if isNonterminalByte(symbol) && !lhsHasRule[symbol] {
				return resp, &validationError{"非终结符 " + string(symbol) + " 缺少产生式"}
			}
		}
	}
	for _, token := range tokens {
		if token == "$" {
			return resp, &validationError{"$ 是保留的结束符，不能作为输入词"}
		}
	}

	nonterminals := sortedByteList(ntSet)
	terminals := sortedTerminalNames(terminalSet)
	terminals = append(terminals, "$")
	resp.Nonterminals = bytesToStrings(nonterminals)
	resp.Terminals = terminals

	nullable := map[byte]bool{}
	rhsFirst := make([][]byte, len(rules))
	rhsNullable := make([]bool, len(rules))

	// 定点迭代：先单独计算可空性，避免把 FOLLOW 的 $ 或 ε 混入 FIRST。
	for changed := true; changed; {
		changed = false
		for _, rule := range rules {
			if nullable[rule.lhs] {
				continue
			}
			canBeEmpty := true
			for i := 0; i < len(rule.rhs); i++ {
				symbol := rule.rhs[i]
				if isTerminalByte(symbol) || !nullable[symbol] {
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

	firstSets := map[byte]map[byte]bool{}
	for _, nt := range nonterminals {
		firstSets[nt] = map[byte]bool{}
	}
	for changed := true; changed; {
		changed = false
		for ruleIndex, rule := range rules {
			first := firstSets[rule.lhs]
			rhsTerminals := []byte{}
			allNullable := len(rule.rhs) == 0
			for i := 0; i < len(rule.rhs); i++ {
				symbol := rule.rhs[i]
				if isTerminalByte(symbol) {
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

	followSets := map[byte]map[byte]bool{}
	for _, nt := range nonterminals {
		followSets[nt] = map[byte]bool{}
	}
	followSets[start[0]]['$'] = true

	for changed := true; changed; {
		changed = false
		addFollow := func(from, terminal byte) {
			if terminal != '$' && !isTerminalByte(terminal) {
				return
			}
			if !followSets[from][terminal] {
				followSets[from][terminal] = true
				changed = true
			}
		}

		for _, rule := range rules {
			rhs := rule.rhs
			for i := 0; i < len(rhs); i++ {
				symbol := rhs[i]
				if !isNonterminalByte(symbol) {
					continue
				}
				betaFirst := []byte{}
				betaNullable := true
				for j := i + 1; j < len(rhs); j++ {
					next := rhs[j]
					if isTerminalByte(next) {
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
		symbol := nt[0]
		resp.First[nt] = SetInfo{
			Terminals: sortedSet(firstSets[symbol]),
			Nullable:  nullable[symbol],
		}
		resp.Follow[nt] = sortedSet(followSets[symbol])
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
			if conflict == nil || symbolPairLess(cell.Nonterminal[0], cell.Terminal[0], conflict.Nonterminal[0], conflict.Terminal[0]) {
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
		return symbolPairLess(table[i].Nonterminal[0], table[i].Terminal[0], table[j].Nonterminal[0], table[j].Terminal[0])
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
			lookup[cell.Nonterminal+cell.Terminal] = cell.RuleIDs[0]
		}
	}
	resp.Steps = runPredictiveParser(start, rules, tokens, lookup, terminalSet)
	resp.Accepted = len(resp.Steps) > 0 && resp.Steps[len(resp.Steps)-1].Action == "accept"
	if !resp.Accepted {
		resp.Message = resp.Steps[len(resp.Steps)-1].Message
	}
	return resp, nil
}

func runPredictiveParser(start string, rules []grammarRule, tokens []string, lookup map[string]int, grammarTerminals map[byte]bool) []ParseStep {
	stack := []byte{'$', start[0]}
	remainingTokens := append([]string(nil), tokens...)
	steps := []ParseStep{}
	snapshot := func(action, label, message string, ruleID *int, ruleText string) ParseStep {
		remaining := strings.Join(remainingTokens, " ")
		if remaining != "" {
			remaining += " "
		}
		remaining += "$"
		stackCopy := append([]byte(nil), stack...)
		for i, j := 0, len(stackCopy)-1; i < j; i, j = i+1, j-1 {
			stackCopy[i], stackCopy[j] = stackCopy[j], stackCopy[i]
		}
		return ParseStep{
			Step:        len(steps) + 1,
			Action:      action,
			ActionLabel: label,
			Stack:       string(stackCopy),
			Remaining:   remaining,
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
		current := currentToken[0]

		if top == '$' && currentToken == "$" {
			steps = append(steps, snapshot("accept", "接受", "", nil, ""))
			return steps
		}

		if currentToken != "$" {
			if len(currentToken) != 1 || !isTerminalByte(current) || !grammarTerminals[current] {
				message := "非法词：" + currentToken
				steps = append(steps, snapshot("error", "错误", message, nil, ""))
				return steps
			}
		} else {
			current = '$'
		}

		if isTerminalByte(top) || top == '$' {
			if top != current {
				expected := displaySymbol(top)
				got := displaySymbol(current)
				message := "匹配失败：期望 " + expected + "，实际遇到 " + got
				steps = append(steps, snapshot("error", "错误", message, nil, ""))
				return steps
			}
			stack = stack[:len(stack)-1]
			remainingTokens = remainingTokens[1:]
			steps = append(steps, snapshot("match", "匹配 "+string(top), "", nil, ""))
			continue
		}

		ruleID, ok := lookup[string(top)+string(current)]
		if !ok {
			message := "分析表 M[" + string(top) + ", " + displaySymbol(current) + "] 为空，无法选择产生式"
			steps = append(steps, snapshot("error", "错误", message, nil, ""))
			return steps
		}

		stack = stack[:len(stack)-1]
		rhs := rules[ruleID].rhs
		for i := len(rhs) - 1; i >= 0; i-- {
			stack = append(stack, rhs[i])
		}
		id := ruleID
		rhsLabel := rhs
		if rhsLabel == "" {
			rhsLabel = "ε"
		}
		steps = append(steps, snapshot("expand", "展开", "", &id, string(rules[ruleID].lhs)+" -> "+rhsLabel))
	}
}

func addTableCell(tableMap map[string]*TableCell, lhs, terminal byte, ruleID int) {
	key := string(lhs) + string(terminal)
	cell := tableMap[key]
	if cell == nil {
		cell = &TableCell{Nonterminal: string(lhs), Terminal: string(terminal)}
		tableMap[key] = cell
	}
	for _, existing := range cell.RuleIDs {
		if existing == ruleID {
			return
		}
	}
	cell.RuleIDs = append(cell.RuleIDs, ruleID)
}

func isNonterminalByte(value byte) bool {
	return value >= 'A' && value <= 'Z'
}

func isTerminalByte(value byte) bool {
	return value >= 'a' && value <= 'z'
}

func isGrammarSymbolString(value string) bool {
	for i := 0; i < len(value); i++ {
		if !isNonterminalByte(value[i]) && !isTerminalByte(value[i]) {
			return false
		}
	}
	return true
}

// Named terminals such as {id} are accepted alongside legacy letters.
func flattenNamedTerminals(value string) (string, bool) {
	var flat strings.Builder
	for i := 0; i < len(value); {
		if value[i] == '{' {
			end := strings.IndexByte(value[i+1:], '}')
			if end < 0 {
				return "", false
			}
			name := value[i+1 : i+1+end]
			if len(name) < 2 || len(name) > 12 {
				return "", false
			}
			for j := 0; j < len(name); j++ {
				if !isTerminalByte(name[j]) {
					return "", false
				}
			}
			flat.WriteString(name)
			i += end + 2
			continue
		}
		if !isTerminalByte(value[i]) && !isNonterminalByte(value[i]) {
			return "", false
		}
		flat.WriteByte(value[i])
		i++
	}
	return flat.String(), true
}

func sortedByteList(values map[byte]bool) []byte {
	list := make([]byte, 0, len(values))
	for value := range values {
		list = append(list, value)
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	return list
}

func sortedTerminalNames(values map[byte]bool) []string {
	result := make([]string, 0, len(values))
	bytesList := make([]byte, 0, len(values))
	for value := range values {
		bytesList = append(bytesList, value)
	}
	sort.Slice(bytesList, func(i, j int) bool { return bytesList[i] < bytesList[j] })
	for _, value := range bytesList {
		result = append(result, string(value))
	}
	return result
}

func bytesToStrings(values []byte) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func sortedSet(values map[byte]bool) []string {
	list := make([]byte, 0, len(values))
	for value := range values {
		list = append(list, value)
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	result := make([]string, 0, len(list))
	for _, value := range list {
		result = append(result, string(value))
	}
	return result
}

func symbolPairLess(aNonterminal, aTerminal, bNonterminal, bTerminal byte) bool {
	if aNonterminal != bNonterminal {
		return aNonterminal < bNonterminal
	}
	return aTerminal < bTerminal
}

func displaySymbol(value byte) string {
	if value == '$' {
		return "结束符 $"
	}
	return string(value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
