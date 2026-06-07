# CODEMAP — generated, do not edit by hand

> Regenerate with `make codemap`. A per-package index of every top-level
> `type`/`func` (with its file and line count) so a session can learn the
> layout from this one file instead of opening source to find a symbol. Read
> this first; jump straight to `file:symbol`. The source is the source of
> truth — if this looks stale, re-run `make codemap`.

_Last generated: 2026-06-07 (UTC)._

## cmd/my-orchestra

### main.go (85 LOC)
- L21: `func main()`
- L49: `func run(configDir, addr string, seed, demo bool) error`
- L67: `func seedStarter(configDir string) error`

## cmd/my-orchestra-desktop

### main.go (104 LOC)
- L34: `func main()`
- L62: `func run(configDir string, demo, serve bool) error`

## internal/bootstrap

### bootstrap.go (373 LOC)
- L34: `func Build(configDir string, demo bool) (srv *web.Hub, close func(), err error)`
- L113: `func demoClient(forge *ctxforge.Forge, spec *copilot.SessionSpec) (copilot.Client, func())`
- L153: `func seedSpend(store *telemetry.SpendStore)`
- L174: `func seedRuns(store *telemetry.RunStore)`
- L202: `func dialClient(cfg *config.Config) (copilot.Client, func())`
- L224: `func ServeLocal(h http.Handler) (port int, stop func(), err error)`
- L236: `func DefaultConfigDir() string`
- L250: `func SeedForge(forge *ctxforge.Forge)`
- L361: `func curatedMCPServers() []ctxforge.MCPServer`

## internal/config

### config.go (178 LOC)
- L15: `type Config struct`
- L48: `type TelemetryConfig struct`
- L65: `func Default(dir string) *Config`
- L83: `func (c *Config) Dir() string { return c.dir }`
- L87: `func Load(dir string) (*Config, error)`
- L110: `func (c *Config) Save() error`
- L131: `func (c *Config) normalize()`
- L142: `func (c *Config) Validate() error`
- L178: `func (c *Config) GitHubToken() string { return os.Getenv(c.GitHubTokenEnv) }`

### keybindings.go (114 LOC)
- L18: `type KeyAction struct`
- L27: `func KeyActions() []KeyAction`
- L40: `type ResolvedKey struct`
- L48: `func (c *Config) Keymap() []ResolvedKey`
- L64: `func knownKeyAction(id string) bool`
- L75: `func (c *Config) normalizeKeyBindings()`
- L92: `func (c *Config) validateKeyBindings() error`

## internal/convo

### convo.go (180 LOC)
- L12: `type Role int`
- L24: `type ToolView struct`
- L36: `type Turn struct`
- L46: `type State struct`
- L54: `func (c *State) AddUser(text string)`
- L59: `func (c *State) AddSystem(text string)`
- L65: `func (c *State) AppendDelta(text string)`
- L72: `func (c *State) AppendReasoning(text string)`
- L78: `func (c *State) commitReasoning()`
- L88: `func (c *State) commitMessage(final string)`
- L101: `func (c *State) Finish(finalContent string)`
- L109: `func (c *State) ToolStart(id, name, args string)`
- L125: `func (c *State) ToolProgress(id, msg string)`
- L132: `func (c *State) ToolEnd(id, result string, success bool)`
- L143: `func (c *State) toolByID(id string) *ToolView`
- L154: `func (c *State) ActiveTools() []string`
- L166: `func (c *State) Committed() []Turn`
- L175: `func (c *State) Pending() (Role, string)`

## internal/copilot

### bridge.go (82 LOC)
- L16: `type bridge[T any] struct`
- L23: `func newBridge[T any](prefix string) *bridge[T]`
- L28: `func (b *bridge[T]) begin() (string, chan T)`
- L40: `func (b *bridge[T]) resolve(id string, v T) bool`
- L53: `func (b *bridge[T]) pending() int`
- L63: `func newPermBridge() *bridge[bool]             { return newBridge[bool]("perm") }`
- L64: `func newInputBridge() *bridge[string]          { return newBridge[string]("ask") }`
- L65: `func newPlanBridge() *bridge[planDecision]     { return newBridge[planDecision]("plan") }`
- L66: `func newElicitBridge() *bridge[elicitDecision] { return newBridge[elicitDecision]("elicit") }`
- L70: `type planDecision struct`
- L79: `type elicitDecision struct`

### copilot.go (279 LOC)
- L15: `type EventType int`
- L44: `type SessionMeta struct`
- L55: `type PermissionRequest struct`
- L67: `type InputRequest struct`
- L79: `type SubagentInfo struct`
- L95: `type ElicitRequest struct`
- L106: `type ElicitField struct`
- L123: `type PlanRequest struct`
- L133: `type UsageData struct`
- L146: `type Event struct`
- L172: `type ContextInfo struct`
- L179: `type ModelInfo struct`
- L188: `type ToolCall struct`
- L205: `type SessionSpec struct`
- L218: `type MCPServer struct`
- L233: `func (m MCPServer) Key() string`
- L241: `type Client interface`

### handlers.go (109 LOC)
- L17: `func (c *SDKClient) permissionHandler() sdk.PermissionHandlerFunc`
- L42: `func (c *SDKClient) userInputHandler() sdk.UserInputHandler`
- L69: `func (c *SDKClient) exitPlanModeHandler() sdk.ExitPlanModeRequestHandler`
- L92: `func (c *SDKClient) elicitationHandler() sdk.ElicitationHandler`

### mock.go (230 LOC)
- L11: `type MockClient struct`
- L43: `func NewMockClient() *MockClient`
- L51: `func (m *MockClient) CreateSession(context.Context, SessionSpec) (string, error)`
- L62: `func (m *MockClient) Send(_ context.Context, _, prompt string, attachments []string, agentMode string) error`
- L76: `func (m *MockClient) SentModeAt(i int) string`
- L83: `func (m *MockClient) Abort(_ context.Context, sessionID string) error`
- L91: `type PermissionDecision struct`
- L97: `func (m *MockClient) Respond(id string, approve bool) error`
- L105: `type InputDecision struct`
- L111: `func (m *MockClient) RespondInput(id, answer string) error`
- L119: `type PlanDecision struct`
- L127: `func (m *MockClient) RespondPlan(id string, approved bool, action, feedback string) error`
- L135: `type ElicitDecision struct`
- L142: `func (m *MockClient) RespondElicit(id, action string, content map[string]any) error`
- L150: `func (m *MockClient) SentCount() int`
- L157: `func (m *MockClient) SentAt(i int) string`
- L164: `func (m *MockClient) ListModels(context.Context) ([]ModelInfo, error)`
- L171: `func (m *MockClient) ListSessions(context.Context) ([]SessionMeta, error)`
- L178: `func (m *MockClient) ResumeSession(_ context.Context, sessionID string, _ SessionSpec) (string, error)`
- L189: `func (m *MockClient) SessionHistory(_ context.Context, sessionID string) ([]Event, error)`
- L199: `func (m *MockClient) DeleteSession(_ context.Context, sessionID string) error`
- L210: `func (m *MockClient) Events() <-chan Event { return m.events }`
- L213: `func (m *MockClient) Emit(e Event)`
- L222: `func (m *MockClient) Close() error`

### normalize.go (431 LOC)
- L25: `func (c *SDKClient) makeHandler(sid string) func(sdk.SessionEvent)`
- L120: `func historyEvents(sid string, raw []sdk.SessionEvent) []Event`
- L150: `func normalizeUsage(d *sdk.AssistantUsageData) UsageData`
- L169: `func normalizeElicitFields(schema *sdk.ElicitationSchema) []ElicitField`
- L194: `func normalizeElicitField(name string, raw any) ElicitField`
- L214: `func elicitStr(m map[string]any, key string) string`
- L223: `func elicitStrSlice(v any) []string`
- L239: `func elicitDefault(v any) string`
- L260: `func sessionError(d *sdk.SessionErrorData) error`
- L272: `func planChangeText(op sdk.PlanChangedOperation) string`
- L287: `func compactionSummary(d *sdk.SessionCompactionCompleteData) string`
- L301: `func describePermission(req sdk.PermissionRequest) string`
- L316: `func permWriteFields(req sdk.PermissionRequest) (file, intention, diff string)`
- L326: `func summarizeArgs(args any) string`
- L348: `func stringField(m map[string]any, key string) (string, bool)`
- L361: `func toolResultText(d *sdk.ToolExecutionCompleteData) string`
- L378: `func oneLine(s string) string`
- L382: `func clip(s string, n int) string`
- L393: `func deref(p *int64) int64`
- L400: `func derefStr(p *string) string`
- L410: `func subagentSummary(durationMs, totalTokens *int64) string`
- L422: `func humanTokenCount(n int64) string`

### sdkclient.go (438 LOC)
- L22: `type SDKClient struct`
- L49: `type Options struct`
- L67: `func ResolveAuth(token string) (githubToken string, useLoggedInUser *bool)`
- L77: `func NewSDKClient(ctx context.Context, opts Options) (*SDKClient, error)`
- L116: `func (c *SDKClient) applyHandlers(autoApprove bool) (onPerm sdk.PermissionHandlerFunc, onInput sdk.UserInputHandler, onPlan sdk.ExitPlanModeRequestHandler, onElicit sdk.ElicitationHandler)`
- L126: `func (c *SDKClient) CreateSession(ctx context.Context, spec SessionSpec) (string, error)`
- L168: `func (c *SDKClient) ListSessions(ctx context.Context) ([]SessionMeta, error)`
- L189: `func (c *SDKClient) ResumeSession(ctx context.Context, sessionID string, spec SessionSpec) (string, error)`
- L216: `func (c *SDKClient) SessionHistory(ctx context.Context, sessionID string) ([]Event, error)`
- L232: `func (c *SDKClient) DeleteSession(ctx context.Context, sessionID string) error`
- L250: `func shouldDropReasoningEffort(effort string, supported []string, known bool) bool`
- L268: `func (c *SDKClient) modelReasoningEfforts(ctx context.Context, model string) (efforts []string, known bool)`
- L291: `func (c *SDKClient) register(session *sdk.Session)`
- L301: `func (c *SDKClient) ListModels(ctx context.Context) ([]ModelInfo, error)`
- L314: `func (c *SDKClient) Respond(id string, approve bool) error`
- L322: `func (c *SDKClient) RespondInput(id, answer string) error`
- L330: `func (c *SDKClient) RespondPlan(id string, approved bool, action, feedback string) error`
- L338: `func (c *SDKClient) RespondElicit(id, action string, content map[string]any) error`
- L346: `func (c *SDKClient) Send(ctx context.Context, sessionID, prompt string, attachments []string, agentMode string) error`
- L366: `func toAgentMode(mode string) sdk.AgentMode`
- L382: `func (c *SDKClient) Abort(ctx context.Context, sessionID string) error`
- L393: `func (c *SDKClient) Events() <-chan Event { return c.events }`
- L396: `func (c *SDKClient) emit(e Event)`
- L404: `func (c *SDKClient) Close() error`

## internal/ctxforge

### forge.go (562 LOC)
- L13: `type Forge struct`
- L29: `func New(dir string) *Forge`
- L35: `func Load(dir string) (*Forge, error)`
- L56: `func (f *Forge) Save() error`
- L79: `func (f *Forge) Validate() error`
- L141: `func uniqueIDs(kind string, n int, id func(int) string) error`
- L153: `func (f *Forge) Skill(id string) *Skill`
- L163: `func (f *Forge) Agent(id string) *Agent`
- L181: `func (f *Forge) HasOwnChatAgent() bool`
- L192: `type forgeState struct`
- L201: `func (f *Forge) snapshot() forgeState`
- L212: `func (f *Forge) restore(s forgeState)`
- L224: `func (f *Forge) mutate(apply func() error) error`
- L238: `func (f *Forge) AddSkill(s Skill) error`
- L252: `func (f *Forge) Instruction(id string) *Instruction`
- L262: `func (f *Forge) AddInstruction(in Instruction) error`
- L277: `func (f *Forge) AddAgent(a Agent) error`
- L293: `func (f *Forge) UpdateSkill(id string, s Skill) error`
- L306: `func (f *Forge) UpdateInstruction(id string, in Instruction) error`
- L319: `func (f *Forge) UpdateAgent(id string, a Agent) error`
- L331: `func (f *Forge) ToggleSkill(id string) (bool, error)`
- L341: `func (f *Forge) ToggleInstruction(id string) (bool, error)`
- L354: `func (f *Forge) RemoveSkill(id string) error`
- L367: `func (f *Forge) RemoveInstruction(id string) error`
- L382: `func (f *Forge) RemoveAgent(id string) error`
- L395: `func (f *Forge) MCPServer(id string) *MCPServer`
- L405: `func (f *Forge) AddMCPServer(m MCPServer) error`
- L421: `func (f *Forge) UpdateMCPServer(id string, m MCPServer) error`
- L433: `func (f *Forge) ToggleMCPServer(id string) (bool, error)`
- L444: `func (f *Forge) RemoveMCPServer(id string) error`
- L457: `type SessionSpec struct`
- L475: `func (f *Forge) Compile(agentID string) (SessionSpec, error)`
- L542: `func (f *Forge) activeSkills(agent *Agent) []Skill`

### snippet.go (87 LOC)
- L14: `type Snippet struct`
- L24: `func (s Snippet) Validate() error`
- L38: `func (f *Forge) Snippet(id string) *Snippet`
- L48: `func (f *Forge) AddSnippet(s Snippet) error`
- L64: `func (f *Forge) UpdateSnippet(id string, s Snippet) error`
- L77: `func (f *Forge) RemoveSnippet(id string) error`

### types.go (146 LOC)
- L22: `type Skill struct`
- L35: `type Instruction struct`
- L45: `type Agent struct`
- L67: `func DefaultChatAgent() Agent`
- L77: `type MCPServer struct`
- L92: `func (s Skill) Validate() error`
- L106: `func (i Instruction) Validate() error`
- L117: `func (a Agent) Validate() error`
- L131: `func (m MCPServer) Validate() error`
- L141: `func validateID(kind, id string) error`

### workflow.go (234 LOC)
- L14: `type Workflow struct`
- L26: `type WorkflowStep struct`
- L44: `type StepCondition struct`
- L83: `func (w Workflow) EffectiveMode() string`
- L92: `func (w Workflow) Validate() error`
- L123: `func (c *StepCondition) validate(wfID string, i int) error`
- L146: `type CompiledStep struct`
- L160: `func (f *Forge) CompileWorkflow(id string) (Workflow, []CompiledStep, error)`
- L183: `func (f *Forge) Workflow(id string) *Workflow`
- L195: `func (f *Forge) AddWorkflow(w Workflow) error`
- L211: `func (f *Forge) UpdateWorkflow(id string, w Workflow) error`
- L224: `func (f *Forge) RemoveWorkflow(id string) error`

## internal/telemetry

### bucketforecast.go (89 LOC)
- L28: `type BucketProjection struct`
- L39: `func DailyTotalsBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) map[string][]DayTotal`
- L68: `func BucketForecasts(records []SpendRecord, budget Budget, now time.Time, keyOf func(SpendRecord) string, includeEmpty bool) []BucketProjection`

### credits.go (293 LOC)
- L11: `type Usage struct`
- L27: `func (u Usage) TotalTokens() int64`
- L32: `type Cost struct`
- L42: `func (c Cost) USD() float64 { return c.InputUSD + c.CachedUSD + c.OutputUSD }`
- L45: `func (c Cost) Credits() float64 { return c.USD() / USDPerCredit }`
- L49: `func Price(pb *PriceBook, u Usage) Cost`
- L67: `func EstimateTurn(pb *PriceBook, model string, contextTokens int64) Cost`
- L77: `type Meter struct`
- L93: `func (m *Meter) RecordReportedAIU(aiu float64)`
- L104: `func (m *Meter) ReportedAIU() float64`
- L111: `type ModelTotals struct`
- L123: `func (m ModelTotals) USD() float64 { return m.InputUSD + m.CachedUSD + m.OutputUSD }`
- L126: `func (m ModelTotals) Credits() float64 { return m.USD() / USDPerCredit }`
- L130: `func NewMeter(pb *PriceBook) *Meter`
- L140: `func (m *Meter) PriceBook() *PriceBook { return m.pb }`
- L144: `func (m *Meter) Record(u Usage) Cost`
- L179: `func (m *Meter) ExtraTokens() (cacheWrite, reasoning int64)`
- L188: `func (m *Meter) EstimateTurn(model string, contextTokens int64) Cost`
- L193: `func (m *Meter) Totals() Cost`
- L203: `func (m *Meter) TotalTokens() (input, cached, output int64)`
- L216: `func (m *Meter) ByModel() []ModelTotals`
- L233: `func (m *Meter) Count() int`
- L242: `type Budget struct`
- L254: `func (b Budget) Remaining(used float64) float64 { return b.AllowanceCredits - used }`
- L258: `func (b Budget) FractionUsed(used float64) float64`
- L271: `func (b Budget) Warned(used float64) bool`
- L281: `func (b Budget) CapExceeded(projected float64) bool`
- L290: `func FormatUSD(v float64) string { return fmt.Sprintf("$%.4f", v) }`
- L293: `func FormatCredits(v float64) string { return fmt.Sprintf("%.2f cr", v) }`

### forecast.go (142 LOC)
- L24: `type ProjectionStatus int`
- L45: `type Projection struct`
- L76: `func Forecast(daily []DayTotal, budget Budget, now time.Time) Projection`

### history.go (398 LOC)
- L29: `type SpendRecord struct`
- L55: `func (r SpendRecord) Credits() float64 { return r.USD / USDPerCredit }`
- L58: `func (r SpendRecord) Day() string { return r.At.UTC().Format("2006-01-02") }`
- L72: `type spendDoc struct`
- L80: `type SpendStore struct`
- L89: `func LoadSpendStore(dir string) (*SpendStore, error)`
- L115: `func (s *SpendStore) Append(r SpendRecord) error`
- L127: `func (s *SpendStore) save() error`
- L148: `func (s *SpendStore) Records() []SpendRecord`
- L157: `func (s *SpendStore) Count() int`
- L164: `type DayTotal struct`
- L173: `func DailyTotals(records []SpendRecord) []DayTotal`
- L211: `func MonthToDate(records []SpendRecord, now time.Time) Cost`
- L225: `type share struct`
- L239: `func shareBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) []share`
- L270: `type ModelShare struct`
- L280: `func ModelShares(records []SpendRecord) []ModelShare`
- L292: `type AgentShare struct`
- L303: `func AgentShares(records []SpendRecord) []AgentShare`
- L314: `type WorkflowShare struct`
- L326: `func WorkflowShares(records []SpendRecord) []WorkflowShare`
- L337: `type SessionShare struct`
- L350: `func SessionShares(records []SpendRecord) []SessionShare`
- L362: `func WriteCSV(w io.Writer, records []SpendRecord) error`
- L396: `func csvFloat(v float64) string`

### pricing.go (188 LOC)
- L27: `type ModelRate struct`
- L51: `type PriceBook struct`
- L60: `func NewPriceBook(fallback ModelRate, rates ...ModelRate) *PriceBook`
- L71: `func DefaultPriceBook() *PriceBook`
- L88: `func (pb *PriceBook) Rate(model string) (ModelRate, bool)`
- L103: `func (pb *PriceBook) Set(r ModelRate)`
- L113: `func (pb *PriceBook) Models() []string`
- L134: `func (pb *PriceBook) Replace(src *PriceBook)`
- L161: `func BuildPriceBook(overrides map[string][3]float64) *PriceBook`
- L177: `func normalizeModel(m string) string`
- L185: `func (r ModelRate) String() string`

### runs.go (276 LOC)
- L27: `type RunLane struct`
- L38: `type RunRecord struct`
- L53: `func (r RunRecord) Credits() float64`
- L65: `func (r RunRecord) Duration() time.Duration`
- L84: `type runDoc struct`
- L92: `type RunStore struct`
- L101: `func LoadRunStore(dir string) (*RunStore, error)`
- L127: `func (s *RunStore) Append(r RunRecord) error`
- L139: `func (s *RunStore) save() error`
- L160: `func (s *RunStore) Records() []RunRecord`
- L169: `func (s *RunStore) Count() int`
- L182: `type RunAggregate struct`
- L205: `func (a RunAggregate) FailureRate() float64`
- L221: `func RunAggregates(records []RunRecord) []RunAggregate`
- L271: `func laterRun(s2, f2, s1, f1 time.Time) bool`

## internal/web

### assets.go (24 LOC)
- _(no top-level type/func declarations)_

### autocomplete.go (161 LOC)
- L16: `type commandSpec struct`
- L38: `func commandSpecs() []commandSpec`
- L50: `func slashPrefix(input string) (q string, ok bool)`
- L65: `func matchCommands(input string) []commandSpec`
- L83: `func matchSnippets(input string, snippets []ctxforge.Snippet) []ctxforge.Snippet`
- L104: `func isReservedCommand(name string) bool`
- L120: `type menuEntry struct`
- L130: `func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request)`
- L149: `func renderMenu(cmds []commandSpec, snippets []ctxforge.Snippet) string`

### commands.go (409 LOC)
- L23: `func parseCommand(input string) (name, args string, ok bool)`
- L34: `func (s *Server) runCommand(input string) string`
- L66: `func (s *Server) systemNote(text string) string`
- L75: `func (s *Server) clearConversation()`
- L100: `func (s *Server) cmdClear() string`
- L118: `func (s *Server) setMode(target, args string) bool`
- L142: `func (s *Server) cmdPlan(args string) string`
- L152: `func (s *Server) cmdAuto(args string) string`
- L161: `func (s *Server) cmdAsk(args string) string`
- L171: `func (s *Server) cmdEffort(arg string) string`
- L193: `func (s *Server) setEffort(effort string)`
- L210: `func (s *Server) cmdModel(name string) string`
- L225: `func (s *Server) setModel(name string)`
- L244: `func (s *Server) cmdAgent(arg string) string`
- L305: `func SeamSpec(cs ctxforge.SessionSpec, defModel, defEffort string, lookupEnv func(string) string) copilot.SessionSpec`
- L322: `func (s *Server) compiledSpec(agentID string) copilot.SessionSpec`
- L336: `func (s *Server) applyAgentSpec(c copilot.SessionSpec, agentID string) string`
- L352: `func (s *Server) cmdCost() string`
- L365: `func (s *Server) cmdAttach(path string) string`
- L379: `func (s *Server) cmdNav(slug string) string`
- L384: `func isNavSlug(slug string) bool`
- L395: `func commandHelp() string`

### demo.go (131 LOC)
- L16: `func streamDemoReply(m *copilot.MockClient, prompt string)`
- L122: `func tokenize(s string) []string`

### diff.go (157 LOC)
- L15: `type diffLineKind int`
- L30: `type diffLine struct`
- L41: `type diffView struct`
- L53: `func parseUnifiedDiff(s string) diffView`
- L107: `func isHunkHeader(s string) bool`
- L117: `func hunkStarts(s string) (oldStart, newStart int)`
- L134: `func leadingInt(s string) int`
- L147: `func isFileHeader(s string) bool`

### forgecrud.go (120 LOC)
- L19: `type forgeCRUD[T any] struct`
- L32: `func (c forgeCRUD[T]) New(s *Server, w http.ResponseWriter, r *http.Request)`
- L38: `func (c forgeCRUD[T]) Edit(s *Server, w http.ResponseWriter, r *http.Request)`
- L55: `func (c forgeCRUD[T]) Create(s *Server, w http.ResponseWriter, r *http.Request)`
- L66: `func (c forgeCRUD[T]) Update(s *Server, w http.ResponseWriter, r *http.Request)`
- L78: `func (c forgeCRUD[T]) Delete(s *Server, w http.ResponseWriter, r *http.Request)`

### forms.go (199 LOC)
- L22: `func textField(label, name, value string, required bool) string`
- L26: `func textArea(label, name, value string, required bool) string`
- L30: `func numberField(label, name string, value int) string`
- L34: `func checkboxField(label, name string, on bool) string`
- L38: `func selectField(label, name, value string, opts []string) string`
- L52: `func idField(value string, isNew bool) string`
- L60: `func formShell(title, action, kind, errMsg string, fields ...string) string`
- L69: `func renderSkillForm(s ctxforge.Skill, isNew bool, errMsg string) string`
- L84: `func (s *Server) handleSkillNew(w http.ResponseWriter, r *http.Request)  { skillCRUD.New(s, w, r) }`
- L85: `func (s *Server) handleSkillEdit(w http.ResponseWriter, r *http.Request) { skillCRUD.Edit(s, w, r) }`
- L87: `func skillFromForm(r *http.Request, id string) ctxforge.Skill`
- L98: `func (s *Server) handleSkillCreate(w http.ResponseWriter, r *http.Request) { skillCRUD.Create(s, w, r) }`
- L99: `func (s *Server) handleSkillUpdate(w http.ResponseWriter, r *http.Request) { skillCRUD.Update(s, w, r) }`
- L103: `func renderInstructionForm(in ctxforge.Instruction, isNew bool, errMsg string) string`
- L117: `func (s *Server) handleInstructionNew(w http.ResponseWriter, r *http.Request)`
- L120: `func (s *Server) handleInstructionEdit(w http.ResponseWriter, r *http.Request)`
- L124: `func instructionFromForm(r *http.Request, id string) ctxforge.Instruction`
- L135: `func (s *Server) handleInstructionCreate(w http.ResponseWriter, r *http.Request)`
- L138: `func (s *Server) handleInstructionUpdate(w http.ResponseWriter, r *http.Request)`
- L146: `func renderAgentForm(a ctxforge.Agent, isNew bool, errMsg string) string`
- L167: `func (s *Server) handleAgentNew(w http.ResponseWriter, r *http.Request)  { agentCRUD.New(s, w, r) }`
- L168: `func (s *Server) handleAgentEdit(w http.ResponseWriter, r *http.Request) { agentCRUD.Edit(s, w, r) }`
- L170: `func agentFromForm(r *http.Request, id string) ctxforge.Agent`
- L183: `func (s *Server) handleAgentCreate(w http.ResponseWriter, r *http.Request) { agentCRUD.Create(s, w, r) }`
- L184: `func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request) { agentCRUD.Update(s, w, r) }`
- L187: `func parseCSV(s string) []string`

### hub.go (294 LOC)
- L28: `type Hub struct`
- L62: `type Options struct`
- L84: `func New(opts Options) *Hub`
- L114: `func (h *Hub) newSession(id string) *Server`
- L144: `func (h *Hub) session(w http.ResponseWriter, r *http.Request) *Server`
- L173: `func (h *Hub) bind(copilotID string, s *Server)`
- L182: `func (h *Hub) route(copilotID string) *Server`
- L198: `func (h *Hub) pump()`
- L208: `func (h *Hub) Handler() http.Handler`
- L287: `func (s *Server) Handler() http.Handler { return s.hub.Handler() }`
- L290: `func newID() string`

### instructions_import.go (65 LOC)
- L31: `func importInstructionFiles(dir string) []ctxforge.Instruction`
- L51: `func (s *Server) handleInstructionImport(w http.ResponseWriter, r *http.Request)`

### markdown.go (219 LOC)
- L32: `func renderMarkdown(src string) string`
- L136: `func isBlockStart(line string) bool`
- L153: `func isHR(line string) bool`
- L173: `func inline(s string) string`
- L211: `func safeURL(url string) bool`

### mcp.go (344 LOC)
- L27: `func MCPServerSpecs(servers []ctxforge.MCPServer, lookupEnv func(string) string) []copilot.MCPServer`
- L48: `func envRef(v string) (string, bool)`
- L65: `func resolveEnv(env map[string]string, lookupEnv func(string) string) map[string]string`
- L102: `func (s *Server) mcpServersPartial() string`
- L127: `func (s *Server) commandAvailable(command string) bool`
- L141: `func (s *Server) missingEnvRefs(m ctxforge.MCPServer) []string`
- L160: `func sortedEnvKeys(env map[string]string) []string`
- L175: `type envRow struct`
- L185: `func renderMCPServerForm(m ctxforge.MCPServer, isNew bool, errMsg string, envRows []envRow) string`
- L203: `func renderMCPEnvEditor(rows []envRow) string`
- L214: `func envRowsFromEnv(env map[string]string) []envRow`
- L228: `func envRowsFromForm(r *http.Request) []envRow`
- L252: `func envFromForm(r *http.Request) (map[string]string, error)`
- L273: `func (s *Server) handleMCPServerNew(w http.ResponseWriter, r *http.Request)`
- L277: `func (s *Server) handleMCPServerEdit(w http.ResponseWriter, r *http.Request)`
- L292: `func mcpServerFromForm(r *http.Request, id string) ctxforge.MCPServer`
- L302: `func (s *Server) handleMCPServerCreate(w http.ResponseWriter, r *http.Request)`
- L316: `func (s *Server) handleMCPServerUpdate(w http.ResponseWriter, r *http.Request)`
- L333: `func (s *Server) handleMCPServerToggle(w http.ResponseWriter, r *http.Request)`
- L339: `func (s *Server) handleMCPServerDelete(w http.ResponseWriter, r *http.Request)`

### pages.go (639 LOC)
- L48: `func clampWindow(raw string) int`
- L64: `func (s *Server) renderPage(slug, window string) string`
- L98: `func renderShortcuts(keymap []config.ResolvedKey) string`
- L112: `func helpOverlay(keymap []config.ResolvedKey) string { return helpOverlayAttr(keymap, "") }`
- L118: `func helpOverlayAttr(keymap []config.ResolvedKey, extraAttr string) string`
- L130: `func keymapJSON(keymap []config.ResolvedKey) string`
- L147: `func keymapLiveApply(keymap []config.ResolvedKey) string`
- L155: `func (s *Server) helpPartial() string`
- L213: `func (s *Server) chatPartial() string`
- L245: `func (s *Server) telemetryPartial(window int) string`
- L303: `func (s *Server) spendTrend(window int) (days, shares []map[string]any, hasHistory bool)`
- L360: `func (s *Server) spendShares(now time.Time) (agents, workflows []map[string]any)`
- L390: `func agentKey(r telemetry.SpendRecord) string    { return r.AgentID }`
- L391: `func workflowKey(r telemetry.SpendRecord) string { return r.WorkflowID }`
- L397: `func bucketTrajectories(bs []telemetry.BucketProjection, now time.Time) map[string]string`
- L413: `func bucketTrajectoryText(p telemetry.Projection, now time.Time) string`
- L432: `func daysLeftInMonth(now time.Time) int`
- L443: `func shareRow(label string, credits, fraction float64) map[string]any`
- L455: `func forecastView(fc telemetry.Projection, allowance float64, now time.Time) map[string]any`
- L479: `func forecastSoon(exhaust, now time.Time) bool`
- L489: `func plural(n int, one, many string) string`
- L499: `func (s *Server) agentLabel(id string) string`
- L511: `func (s *Server) workflowLabel(id string) string`
- L521: `func (s *Server) handleSpendExport(w http.ResponseWriter, r *http.Request)`
- L533: `func (s *Server) skillsPartial() string`
- L545: `func (s *Server) instructionsPartial() string`
- L558: `func (s *Server) agentsPartial() string`
- L582: `func addData(kind, noun string) map[string]any { return map[string]any{"Kind": kind, "Noun": noun} }`
- L588: `func (s *Server) modelsPartial() string`
- L618: `func (s *Server) settingsPartial() string`
- L622: `func def(s, fallback string) string`
- L630: `func truncate(s string, n int) string`

### render.go (452 LOC)
- L25: `func esc(s string) string`
- L31: `func deltaSpan(text string) string { return frag("deltaSpan", text) }`
- L34: `func renderTurn(t convo.Turn) string`
- L57: `func renderToolCard(tv *convo.ToolView) string`
- L77: `func renderCur(role convo.Role, text string) string`
- L87: `func renderTimelineInner(st *convo.State) string`
- L110: `func renderPermForm(req copilot.PermissionRequest) string`
- L125: `func diffLineViews(lines []diffLine) []map[string]any`
- L148: `func diffClass(k diffLineKind) string`
- L165: `func diffMarker(k diffLineKind) string`
- L181: `func diffLabel(k diffLineKind) string`
- L193: `func gutterNum(n int) string`
- L203: `func renderAskForm(req copilot.InputRequest) string`
- L213: `func renderPlanForm(req copilot.PlanRequest) string`
- L223: `func renderElicitForm(req copilot.ElicitRequest) string`
- L235: `func elicitFieldView(f copilot.ElicitField) map[string]any`
- L260: `func elicitFieldKey(name string) string { return "f." + name }`
- L264: `func subagentLabel(sa copilot.SubagentInfo) string`
- L279: `func renderSubagents(active []copilot.SubagentInfo) string`
- L300: `func renderStatus(text string, active bool, startMs int64) string`
- L308: `func renderCtx(cur, limit int64, compacting bool) string`
- L343: `func renderStatline(s *Server) string`
- L390: `func statlineForecast(s *Server, now time.Time) (show bool, short string, warn bool, title string)`
- L404: `func humanTokens(n int64) string`
- L420: `func renderCostFooter(credits float64, budget telemetry.Budget) string`
- L437: `func renderBudgetForm(projected, capCredits float64) string`
- L446: `func clampLines(s string, n int) string`

### runs.go (120 LOC)
- L23: `func (s *Server) runsPartial() string`
- L44: `func (s *Server) runSummaryRow(a telemetry.RunAggregate) map[string]any`
- L59: `func (s *Server) runRow(r telemetry.RunRecord) map[string]any`
- L87: `func humanDuration(d time.Duration) string`
- L115: `func runOutcomeGlyph(outcome string) (glyph, state string)`

### server.go (880 LOC)
- L27: `type Server struct`
- L94: `func (s *Server) subscribe() chan fragment`
- L103: `func (s *Server) unsubscribe(ch chan fragment)`
- L114: `func (s *Server) broadcast(frags []fragment)`
- L133: `func (s *Server) broadcastSendFailure(err error)`
- L138: `type indexData struct`
- L148: `type navItem struct`
- L152: `func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request)`
- L174: `func (s *Server) ensureSession(ctx context.Context) (string, error)`
- L193: `func (s *Server) handleSend(w http.ResponseWriter, r *http.Request)`
- L288: `func (s *Server) overCap() (projected float64, capped bool)`
- L306: `func (s *Server) monthToDate() telemetry.Cost`
- L318: `func (s *Server) forecast(now time.Time) (telemetry.Projection, bool)`
- L331: `func (s *Server) budget() telemetry.Budget`
- L341: `type budgetGate struct`
- L351: `func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request)`
- L407: `func (s *Server) dispatch(ctx context.Context, sessionID, prompt string, attachments []string) error`
- L426: `func (s *Server) sendFailedOOB(err error) string`
- L438: `func queuedStatus(n int) string`
- L445: `func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request)`
- L466: `func (s *Server) handlePerm(w http.ResponseWriter, r *http.Request)`
- L496: `func (s *Server) dropLanePerm(id string) string`
- L513: `func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request)`
- L533: `func (s *Server) handlePlanReview(w http.ResponseWriter, r *http.Request)`
- L566: `func (s *Server) handleElicit(w http.ResponseWriter, r *http.Request)`
- L604: `func elicitContent(fields []copilot.ElicitField, form url.Values) map[string]any`
- L634: `func (s *Server) handlePage(w http.ResponseWriter, r *http.Request)`
- L647: `func (s *Server) handleSkillToggle(w http.ResponseWriter, r *http.Request)`
- L653: `func (s *Server) handleSkillDelete(w http.ResponseWriter, r *http.Request)`
- L657: `func (s *Server) handleInstructionToggle(w http.ResponseWriter, r *http.Request)`
- L663: `func (s *Server) handleInstructionDelete(w http.ResponseWriter, r *http.Request)`
- L667: `func (s *Server) handleAgentSelect(w http.ResponseWriter, r *http.Request)`
- L691: `func (s *Server) handleModelSelect(w http.ResponseWriter, r *http.Request)`
- L698: `func (s *Server) handleEffortSelect(w http.ResponseWriter, r *http.Request)`
- L707: `func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request)`
- L730: `func (s *Server) oobTimeline() string`
- L735: `func oobStatus(text string, active bool, startMs int64) string`
- L741: `func (s *Server) oobBudget() string`
- L747: `func (s *Server) budgetFrag() fragment`
- L753: `func (s *Server) renderGate() string`
- L762: `func (s *Server) statusFrag(text string, active bool) fragment`
- L772: `func (s *Server) ctxFrag() fragment`
- L778: `func (s *Server) statFrag() fragment`
- L785: `func (s *Server) oobStat() string`
- L791: `func (s *Server) subagentsFrag() fragment`
- L796: `func nowMs() int64 { return time.Now().UnixMilli() }`
- L801: `func dropByID[T any](sl []T, id string, key func(T) string) []T`
- L812: `func findByID[T any](sl []T, id string, key func(T) string) (T, bool)`
- L822: `func permID(p copilot.PermissionRequest) string { return p.ID }`
- L823: `func inputID(p copilot.InputRequest) string     { return p.ID }`
- L824: `func planID(p copilot.PlanRequest) string       { return p.ID }`
- L825: `func elicitID(e copilot.ElicitRequest) string   { return e.ID }`
- L826: `func subagentKey(a copilot.SubagentInfo) string { return a.ToolCallID }`
- L831: `func (s *Server) dropPerm(id string)  { s.perms = dropByID(s.perms, id, permID) }`
- L832: `func (s *Server) dropInput(id string) { s.inputs = dropByID(s.inputs, id, inputID) }`
- L833: `func (s *Server) dropPlan(id string)  { s.plans = dropByID(s.plans, id, planID) }`
- L834: `func (s *Server) dropElicit(id string)`
- L840: `func (s *Server) findElicit(id string) (copilot.ElicitRequest, bool)`
- L845: `func (s *Server) dropSubagent(toolCallID string)`
- L850: `func firstNonEmpty(vals []string) string`
- L863: `func (s *Server) editForge(fn func() error) error`
- L877: `func (s *Server) writePartial(w http.ResponseWriter, html string)`

### session.go (325 LOC)
- L15: `type liveKind int`
- L28: `func (s *Server) handleEvent(e copilot.Event) []fragment`
- L256: `type spendTag struct`
- L272: `func (s *Server) recordUsage(u copilot.UsageData, tag spendTag) telemetry.Cost`
- L307: `func (s *Server) timelineFragments() []fragment`
- L320: `func toolID(e copilot.Event) string`

### sessions.go (182 LOC)
- L21: `func (s *Server) sessionsPartial() string`
- L33: `func (s *Server) sessionRows(metas []copilot.SessionMeta) []map[string]any`
- L73: `func sessionTitle(m copilot.SessionMeta) string`
- L85: `func humanWhen(t time.Time) string`
- L103: `func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request)`
- L114: `func (s *Server) handleSessionResume(w http.ResponseWriter, r *http.Request)`
- L141: `func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request)`
- L151: `func (s *Server) sessionsError(msg string) string`
- L160: `func (s *Server) loadHistory(events []copilot.Event)`

### settings.go (321 LOC)
- L26: `func (s *Server) editConfig(fn func(*config.Config)) error`
- L41: `func (s *Server) refreshBudget()`
- L55: `func (s *Server) renderSettings(note, errMsg string) string`
- L62: `func renderSettingsForm(c *config.Config, note, errMsg string) string`
- L105: `func priceOverrideFields(c *config.Config) []string`
- L139: `func priceRowField(i int, model string, ov [3]float64, has bool, def telemetry.ModelRate) string`
- L162: `func parsePriceOverrides(r *http.Request) map[string][3]float64`
- L195: `func rateOrDefault(s string, def float64) float64`
- L210: `func formHasPriceOverrides(r *http.Request) bool`
- L223: `func formHasKeyBindings(r *http.Request) bool`
- L234: `func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request)`

### snippets.go (82 LOC)
- L20: `func (s *Server) snippetsPartial() string`
- L37: `func (s *Server) snippetExpansion(name, args string) (string, bool)`
- L51: `func renderSnippetForm(sn ctxforge.Snippet, isNew bool, errMsg string) string`
- L63: `func (s *Server) handleSnippetNew(w http.ResponseWriter, r *http.Request)  { snippetCRUD.New(s, w, r) }`
- L64: `func (s *Server) handleSnippetEdit(w http.ResponseWriter, r *http.Request) { snippetCRUD.Edit(s, w, r) }`
- L66: `func snippetFromForm(r *http.Request, id string) ctxforge.Snippet`
- L74: `func (s *Server) handleSnippetCreate(w http.ResponseWriter, r *http.Request)`
- L77: `func (s *Server) handleSnippetUpdate(w http.ResponseWriter, r *http.Request)`
- L80: `func (s *Server) handleSnippetDelete(w http.ResponseWriter, r *http.Request)`

### sse.go (79 LOC)
- L11: `type fragment struct`
- L21: `func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request)`
- L62: `func writeSSE(w io.Writer, event, data string)`

### tmpl.go (55 LOC)
- L44: `func trusted(s string) template.HTML { return template.HTML(s) } //nolint:gosec // composed from escaped fragments`
- L49: `func frag(name string, data any) string`

### workflow.go (1023 LOC)
- L29: `type laneStatus int`
- L42: `func settled(st laneStatus) bool`
- L48: `type lane struct`
- L68: `func (l *lane) toolStart(id, name, args string)`
- L82: `func (l *lane) toolProgress(id, msg string)`
- L89: `func (l *lane) toolEnd(id, result string, success bool)`
- L100: `func (l *lane) toolByID(id string) *convo.ToolView`
- L112: `func (l *lane) dropPerm(id string) bool`
- L126: `type workflowRun struct`
- L144: `func newWorkflowRun(wf ctxforge.Workflow, steps []ctxforge.CompiledStep, specs []copilot.SessionSpec) *workflowRun`
- L159: `func (r *workflowRun) start() []int`
- L178: `func (r *workflowRun) evalWhen(l *lane) (satisfied, ready bool)`
- L199: `func skipDetail(w *ctxforge.StepCondition) string`
- L218: `func (r *workflowRun) evalPending() []int`
- L249: `func (r *workflowRun) handoffPrompt(idx int) string`
- L269: `func (r *workflowRun) laneFor(sessionID string) *lane`
- L290: `func (l *lane) appendText(s string) { l.text += s }`
- L294: `func (r *workflowRun) finishLane(l *lane, detail string) []int`
- L306: `func (r *workflowRun) failLane(l *lane, msg string) []int`
- L322: `func (r *workflowRun) advance(l *lane) []int`
- L347: `func (r *workflowRun) allSettled() bool`
- L362: `func (s *Server) handleWorkflowRun(w http.ResponseWriter, r *http.Request)`
- L408: `func (s *Server) workflowLaneSpec(cs ctxforge.SessionSpec) copilot.SessionSpec`
- L417: `func (s *Server) launchLanes(run *workflowRun, idxs []int)`
- L425: `func (s *Server) startLane(run *workflowRun, idx int)`
- L455: `func (s *Server) laneError(run *workflowRun, l *lane, err error)`
- L469: `func (s *Server) handleRunEvent(run *workflowRun, e copilot.Event) []fragment`
- L569: `func (s *Server) runFrags(run *workflowRun, done bool) []fragment`
- L594: `func (s *Server) recordRun(run *workflowRun)`
- L607: `func runRecord(run *workflowRun) telemetry.RunRecord`
- L627: `func laneStatusName(st laneStatus) string`
- L643: `func (l *lane) costDetail() string`
- L651: `func (s *Server) lanesFrag() fragment`
- L658: `func renderLanes(run *workflowRun) string`
- L682: `func laneToolsHTML(tools []*convo.ToolView) template.HTML`
- L693: `func lanePermsHTML(perms []copilot.PermissionRequest) template.HTML`
- L704: `func laneGlyph(st laneStatus) (glyph, state string)`
- L711: `func glyphFor(status string) (glyph, state string)`
- L734: `func streamDemoLane(m *copilot.MockClient, sid, prompt string)`
- L776: `func firstLine(s string) string`
- L787: `func (s *Server) workflowsPartial() string`
- L843: `func renderWorkflowForm(w ctxforge.Workflow, isNew bool, errMsg string) string`
- L864: `func stepsToText(steps []ctxforge.WorkflowStep) string`
- L884: `func stepsFromText(raw string) []ctxforge.WorkflowStep`
- L913: `func splitStepLine(line string) (head, prompt string)`
- L932: `func formatStepCondition(c *ctxforge.StepCondition) string`
- L950: `func parseStepCondition(spec string) *ctxforge.StepCondition`
- L970: `func workflowFromForm(r *http.Request, id string) ctxforge.Workflow`
- L980: `func (s *Server) handleWorkflowNew(w http.ResponseWriter, r *http.Request)`
- L984: `func (s *Server) handleWorkflowEdit(w http.ResponseWriter, r *http.Request)`
- L999: `func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request)`
- L1008: `func (s *Server) handleWorkflowUpdate(w http.ResponseWriter, r *http.Request)`
- L1018: `func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request)`

