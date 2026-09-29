<template>
  <main class="page-shell">
    <header class="hero">
      <div>
        <p class="eyebrow">FIRST / FOLLOW / LL(1)</p>
        <h1>预测分析表教学台</h1>
        <p class="subtitle">
          ε 只通过空右部进入 FIRST 的 nullable；FOLLOW 与 FIRST 分开迭代，冲突时只报告竞争规则，不伪造解析栈。
        </p>
      </div>
      <div class="request-card" aria-label="当前请求版本">
        <span>当前请求版本</span>
        <strong data-testid="active-request-id">{{ activeRequestId || '尚未发送' }}</strong>
      </div>
    </header>

    <section class="editor-grid">
      <form class="panel editor" @submit.prevent="analyze">
        <div class="panel-title">
          <h2>文法编辑器</h2>
          <button type="button" class="ghost" @click="loadExample('expression')">表达式文法</button>
          <button type="button" class="ghost" @click="loadExample('conflict')">FOLLOW 冲突</button>
          <button type="button" class="ghost" @click="loadExample('named')">多字符终结符</button>
        </div>

        <label>
          开始符
          <input v-model="start" maxlength="1" aria-label="开始符" data-testid="start-input" placeholder="如 E" />
        </label>

        <label>
          产生式（每行一条，支持 A-&gt;α 或 A→α；空右部为 ε）
          <textarea
            v-model="productionText"
            rows="9"
            spellcheck="false"
            aria-label="产生式"
            data-testid="productions-input"
            placeholder="E->TR&#10;R->pTR&#10;R->"
          ></textarea>
        </label>

        <label>
          词序列（逗号或空格分隔；留空表示空串，最多 40 项）
          <input v-model="tokenText" aria-label="词序列" data-testid="tokens-input" placeholder="i + i" />
        </label>

        <div class="actions">
          <button type="submit" data-testid="analyze-button" :disabled="loading">
            {{ loading ? '计算中…' : '计算 FIRST / FOLLOW / LL(1)' }}
          </button>
          <button type="button" class="secondary" @click="clearResult">清空结果</button>
        </div>

        <p v-if="requestError" class="error" data-testid="request-error" role="alert">{{ requestError }}</p>
      </form>

      <aside class="panel limits">
        <h2>输入约束</h2>
        <ul>
          <li>1～15 个大写非终结符 A-Z</li>
          <li>至多 30 条产生式</li>
          <li>终结符为小写字母 a-z</li>
          <li>可用花括号写 2～12 位小写名称，如 {id}、{num}</li>
          <li>空右部明确表示 ε</li>
          <li>词序列不超过 40 项，$ 为内部结束符</li>
        </ul>
        <div class="formula">
          <strong>构造规则</strong>
          <code>A→α：FIRST(α) 入表</code>
          <code>α⇒ε：FOLLOW(A) 入表</code>
        </div>
      </aside>
    </section>

    <section v-if="result" class="result" :key="result.requestId">
      <div class="version-banner" data-testid="version-banner" :class="{ stale: lastEchoedId !== result.requestId }">
        <span>服务端版本：<strong data-testid="server-request-id">{{ result.requestId }}</strong></span>
        <span v-if="lastEchoedId === result.requestId">这是最近一次请求的结果</span>
        <span v-else>旧响应已停止渲染</span>
      </div>

      <div v-if="result.conflict" class="panel conflict-panel" data-testid="conflict-panel" role="alert">
        <h2 data-testid="conflict-cell">LL(1) 冲突：M[{{ result.conflict.nonterminal }}, {{ result.conflict.terminal }}]</h2>
        <p>按非终结符、终结符字节序选取的最小冲突格包含 {{ result.conflict.ruleIds.map(ruleNumber).join('、') }} 条竞争规则。</p>
        <ul>
          <li v-for="id in result.conflict.ruleIds" :key="id">({{ ruleNumber(id) }}) {{ formatRule(result.productions[id]) }}</li>
        </ul>
        <p class="muted">存在冲突时不预测、不回放，也不会生成看似成功的解析结果。</p>
      </div>

      <div class="panel result-summary">
        <h2>{{ result.conflict ? '仍可检查集合与完整表' : (result.accepted ? '词序列接受' : '词序列拒绝') }}</h2>
        <p v-if="!result.conflict && result.accepted" class="success">栈与输入同时到达 $，分析成功。</p>
        <p v-else-if="!result.conflict" class="error">{{ result.message }}（首个失败步骤为 {{ failureStep }}）</p>
        <p v-else class="warning">{{ result.message }}</p>
      </div>

      <div class="panel">
        <h2>FIRST 与 FOLLOW</h2>
        <div class="sets-grid">
          <div v-for="nt in result.nonterminals" :key="nt" class="set-card">
            <h3>{{ nt }}</h3>
            <p><b>FIRST</b>：{{ formatTerminalSet(result.first[nt].terminals, result.first[nt].nullable) }}</p>
            <p><b>FOLLOW</b>：{{ formatTerminalSet(result.follow[nt], false) }}</p>
          </div>
        </div>
      </div>

      <div class="panel table-panel">
        <h2>LL(1) 预测分析表</h2>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th></th>
                <th v-for="terminal in result.terminals" :key="terminal">{{ terminal }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="nt in result.nonterminals" :key="nt">
                <th>{{ nt }}</th>
                <td v-for="terminal in result.terminals" :key="terminal" :class="{ conflict: cell(nt, terminal)?.ruleIds.length > 1 }">
                  <template v-if="cell(nt, terminal)">
                    <span v-for="id in cell(nt, terminal).ruleIds" :key="id" class="rule-chip">
                      {{ ruleNumber(id) }}
                    </span>
                  </template>
                  <span v-else class="muted">—</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-if="!result.conflict" class="panel steps-panel">
        <h2>栈回放</h2>
        <div class="table-wrap">
          <table data-testid="parse-steps">
            <thead>
              <tr>
                <th>步骤</th>
                <th>动作</th>
                <th>分析栈（顶在左）</th>
                <th>剩余输入</th>
                <th>所用规则</th>
                <th>说明</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="step in result.steps" :key="step.step" :class="{ failed: step.action === 'error' }">
                <td>{{ step.step }}</td>
                <td>{{ step.actionLabel }}</td>
                <td class="mono">{{ step.stack }}</td>
                <td class="mono">{{ step.remaining }}</td>
                <td>
                  <span v-if="step.usedRuleId !== null && step.usedRuleId !== undefined">
                    ({{ ruleNumber(step.usedRuleId) }}) {{ step.rule }}
                  </span>
                </td>
                <td>{{ step.message }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>
  </main>
</template>

<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'

const API_BASE = import.meta.env.VITE_API_BASE_URL || ''
const examples = {
  expression: {
    start: 'E',
    productions: ['E->TR', 'R->pTR', 'R->', 'T->i', 'T->oEc'],
    tokens: ['i', 'p', 'o'],
  },
  conflict: {
    start: 'S',
    productions: ['S->Aa', 'A->a', 'A->'],
    tokens: [],
  },
  named: {
    start: 'S',
    productions: ['S->{id}R', 'R->{plus}{id}R', 'R->'],
    tokens: ['id', 'plus', 'id'],
  },
}

const start = ref('E')
const productionText = ref(examples.expression.productions.join('\n'))
const tokenText = ref('i + i')
const result = ref(null)
const requestError = ref('')
const loading = ref(false)
const activeRequestId = ref('')
const lastEchoedId = ref('')
let abortController = null
let requestSeq = 0

onBeforeUnmount(() => abortController?.abort())

const failureStep = computed(() => {
  const failed = result.value?.steps?.find((step) => step.action === 'error')
  return failed ? failed.step : '-'
})

function normalizeProductions(value) {
  return value
    .split(/\r?\n/)
    .map((line) => line.trim().replace(/→/g, '->'))
    .filter(Boolean)
}

function splitTokens(value) {
  return value.split(/[\s,]+/).map((token) => token.trim()).filter(Boolean)
}

function makeRequestId(seq) {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return `edit-${seq}-${crypto.randomUUID().slice(0, 8)}`
  }
  return `edit-${seq}-${Date.now()}`
}

async function analyze() {
  abortController?.abort()
  abortController = new AbortController()
  const seq = ++requestSeq
  const requestId = makeRequestId(seq)

  requestError.value = ''
  result.value = null
  activeRequestId.value = requestId
  loading.value = true

  const payload = {
    start: start.value.trim(),
    productions: normalizeProductions(productionText.value),
    tokens: splitTokens(tokenText.value),
    requestId,
  }

  try {
    const response = await fetch(`${API_BASE}/api/analyze`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Request-ID': requestId },
      body: JSON.stringify(payload),
      signal: abortController.signal,
    })
    const data = await response.json()
    // 编辑后旧响应即使晚到，也不会替换当前请求版本的服务端推导。
    if (seq !== requestSeq) return
    if (!response.ok) {
      requestError.value = data.error || '请求失败'
      return
    }
    if (data.requestId !== requestId) {
      requestError.value = '服务端响应版本与当前请求不一致'
      return
    }
    result.value = data
    lastEchoedId.value = data.requestId
  } catch (error) {
    if (error.name === 'AbortError' || seq !== requestSeq) return
    requestError.value = error.message || '无法连接 grammar 服务'
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

function clearResult() {
  abortController?.abort()
  requestSeq += 1
  result.value = null
  requestError.value = ''
  activeRequestId.value = ''
  lastEchoedId.value = ''
  loading.value = false
}

function loadExample(name) {
  const example = examples[name]
  start.value = example.start
  productionText.value = example.productions.join('\n')
  tokenText.value = example.tokens.join(' ')
}

function cell(nonterminal, terminal) {
  return result.value?.table?.find((item) => item.nonterminal === nonterminal && item.terminal === terminal)
}

function ruleNumber(id) {
  return Number(id) + 1
}

function formatRule(rule) {
  return rule.replace('->', ' → ').replace(/→\s*$/, '→ ε')
}

function formatTerminalSet(values, nullable) {
  const rendered = [...(values || [])]
  if (nullable) values.push('ε')
  return values.length ? `{ ${values.join(', ')} }` : '∅'
}
</script>
