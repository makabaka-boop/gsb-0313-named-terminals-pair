# LL(1) 预测分析教学台

一个把 **FIRST、FOLLOW、ε、LL(1) 冲突和预测分析栈回放** 分开呈现的教学应用：

- Vue 3 负责编辑文法、开始符和词序列，并展示服务端返回的当前版本结果；
- Go HTTP 服务迭代计算可空性、FIRST/FOLLOW 和 LL(1) 预测分析表；
- Docker Compose 启动 `grammar`（Go API）与 `desk`（Vue/Nginx）两个服务。

## 文法约定

- 非终结符：大写英文字母 `A-Z`，全文法 1～15 个；
- 终结符：小写英文字母 `a-z`；
- 也可在右部使用花括号中的 2～12 位小写终结符名称（如 `{id}`、`{plus}`）；这是一个完整词，不等同于相邻单字母词。词序列按空格或逗号分隔，传给 API 的每项是去掉花括号的名称。
- 产生式格式：`A->α`，前端也支持把 `→` 自动规范化为 `->`；
- 空右部表示 ε，例如 `A->`；
- 产生式至多 30 条；
- 开始符必须是某个产生式左部；
- 输入词序列至多 40 项，`$` 是服务端内部结束符，不能作为输入词。

## 计算语义

1. 先通过定点迭代求非终结符可空性；
2. 再迭代求 FIRST，ε 不放入终结符集合，而用 `nullable: true` 表示；
3. FOLLOW 从开始符的 `$` 开始独立迭代，不把 `$` 或 ε 混入 FIRST；
4. 对每条规则 `A->α`：
   - `FIRST(α)` 中的每个终结符令 `M[A, terminal]` 选择该规则；
   - 当 `α⇒ε` 时，对 `FOLLOW(A)` 中每个终结符（含 `$`）填入该规则；
5. 若一个表格单元有多条规则：
   - 返回按非终结符、终结符字节序最小的冲突格；
   - 返回该格全部竞争规则；
   - 不执行解析、不伪造栈回放；
6. 无冲突时执行预测分析，逐步记录分析栈（栈顶在左，`$` 在右）、剩余输入、动作和所用规则；
7. 非法词、表格空单元或终结符匹配失败都会停在第一个失败步骤。

## 本地开发

### 启动 Go 服务

```bash
cd backend
go test ./...
go run .
```

默认监听 `:8080`。

### 启动 Vue 开发服务器

```bash
cd frontend
npm install
npm run dev
```

Vite 会把 `/api` 代理到 `http://localhost:8080`。打开 http://localhost:5173 。

### 接口示例

```bash
curl -X POST http://localhost:8080/api/analyze \
  -H 'Content-Type: application/json' \
  -d '{
    "start": "E",
    "productions": ["E->TR", "R->pTR", "R->", "T->i", "T->oEc"],
    "tokens": ["i", "p", "o"],
    "requestId": "example-1"
  }'
```

`requestId` 用于标识页面当前编辑版本。服务端会原样回显；前端只渲染与当前请求一致的响应，旧请求的迟到结果不会覆盖页面。

## Docker Compose

```bash
docker compose up --build
```

- `grammar`: http://localhost:8080
- `desk`: http://localhost:5173

`desk` 的 Nginx 会把 `/api/` 与 `/healthz` 代理到 `grammar` 服务。

## 测试

```bash
# Go：间接 ε 传播、FOLLOW 入表冲突、递归文法定点迭代、非法词失败步骤
cd backend
go test ./...

# Playwright：编辑文法 -> 计算 -> 查看第一个失败栈 -> 切换到冲突文法
cd frontend
npx playwright install chromium
npm run dev
npx playwright test
```

Playwright 使用 `PLAYWRIGHT_BASE_URL` 时可覆盖默认地址 `http://localhost:5173`。
