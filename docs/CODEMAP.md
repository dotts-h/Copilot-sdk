# CODEMAP — generated, do not edit by hand

> Regenerate with `make codemap`. A per-package index of every top-level
> `type`/`func` (with its file and line count) so a session can learn the
> layout from this one file instead of opening source to find a symbol. Read
> this first; jump straight to `file:symbol`. The source is the source of
> truth — if this looks stale, re-run `make codemap`.

_Last generated: 2026-06-10 (UTC)._

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

### bootstrap.go (412 LOC)
- L36: `func Build(configDir string, demo bool) (srv *web.Hub, close func(), err error)`
- L122: `func demoClient(forge *ctxforge.Forge, spec *copilot.SessionSpec) (copilot.Client, func())`
- L167: `func seedSpend(store *telemetry.SpendStore)`
- L197: `func seedRuns(store *telemetry.RunStore)`
- L225: `func dialClient(cfg *config.Config) (copilot.Client, func())`
- L248: `func ghCLIToken() (string, error)`
- L263: `func ServeLocal(h http.Handler) (port int, stop func(), err error)`
- L275: `func DefaultConfigDir() string`
- L289: `func SeedForge(forge *ctxforge.Forge)`
- L400: `func curatedMCPServers() []ctxforge.MCPServer`

## internal/config

### config.go (204 LOC)
- L15: `type Config struct`
- L55: `type TelemetryConfig struct`
- L77: `func Default(dir string) *Config`
- L95: `func (c *Config) Dir() string { return c.dir }`
- L99: `func Load(dir string) (*Config, error)`
- L122: `func (c *Config) Save() error`
- L143: `func (c *Config) normalize()`
- L155: `func (c *Config) Validate() error`
- L204: `func (c *Config) GitHubToken() string { return os.Getenv(c.GitHubTokenEnv) }`

### keybindings.go (114 LOC)
- L18: `type KeyAction struct`
- L27: `func KeyActions() []KeyAction`
- L40: `type ResolvedKey struct`
- L48: `func (c *Config) Keymap() []ResolvedKey`
- L64: `func knownKeyAction(id string) bool`
- L75: `func (c *Config) normalizeKeyBindings()`
- L92: `func (c *Config) validateKeyBindings() error`

## internal/convo

### convo.go (234 LOC)
- L12: `type Role int`
- L26: `type ToolView struct`
- L41: `type DecisionView struct`
- L54: `type HookRunView struct`
- L67: `type Turn struct`
- L79: `type State struct`
- L87: `func (c *State) AddUser(text string)`
- L92: `func (c *State) AddSystem(text string)`
- L98: `func (c *State) AppendDelta(text string)`
- L105: `func (c *State) AppendReasoning(text string)`
- L111: `func (c *State) commitReasoning()`
- L121: `func (c *State) commitMessage(final string)`
- L134: `func (c *State) Finish(finalContent string)`
- L142: `func (c *State) ToolStart(id, name, args string)`
- L160: `func (c *State) AddDecision(d DecisionView)`
- L174: `func (c *State) AddHookRun(h HookRunView)`
- L179: `func (c *State) ToolProgress(id, msg string)`
- L186: `func (c *State) ToolEnd(id, result string, success bool)`
- L197: `func (c *State) toolByID(id string) *ToolView`
- L208: `func (c *State) ActiveTools() []string`
- L220: `func (c *State) Committed() []Turn`
- L229: `func (c *State) Pending() (Role, string)`

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

### copilot.go (358 LOC)
- L17: `type EventType int`
- L48: `type SessionMeta struct`
- L59: `type PermissionRequest struct`
- L76: `type InputRequest struct`
- L88: `type SubagentInfo struct`
- L104: `type ElicitRequest struct`
- L115: `type ElicitField struct`
- L132: `type PlanRequest struct`
- L142: `type UsageData struct`
- L155: `type Event struct`
- L195: `type HookRun struct`
- L211: `type ToolDecision struct`
- L221: `type ContextInfo struct`
- L228: `type ModelInfo struct`
- L239: `type AuthStatus struct`
- L250: `type ToolCall struct`
- L267: `type SessionSpec struct`
- L293: `type MCPServer struct`
- L308: `func (m MCPServer) Key() string`
- L316: `type Client interface`

### handlers.go (197 LOC)
- L31: `func (c *SDKClient) permissionHandler() sdk.PermissionHandlerFunc`
- L99: `func isBuiltinHook(id string) bool { return strings.HasPrefix(id, "builtin-") }`
- L105: `func (c *SDKClient) emitDecision(sessionID, kind string, d ctxforge.Decision, req sdk.PermissionRequest)`
- L116: `func permCommand(req sdk.PermissionRequest) string`
- L130: `func (c *SDKClient) userInputHandler() sdk.UserInputHandler`
- L157: `func (c *SDKClient) exitPlanModeHandler() sdk.ExitPlanModeRequestHandler`
- L180: `func (c *SDKClient) elicitationHandler() sdk.ElicitationHandler`

### hookexec.go (194 LOC)
- L40: `type commandRunner func(ctx context.Context, dir, name string, args []string) (output string, exitCode int, err error)`
- L50: `func (c *SDKClient) resolveVarRefs(s string) string`
- L62: `func (c *SDKClient) hookTimeoutOrDefault() time.Duration`
- L74: `func (c *SDKClient) firePostToolHooks(sid string, hooks []ctxforge.Hook, workspace string)`
- L86: `func (c *SDKClient) runPostToolHook(h ctxforge.Hook, workspace string) Event`
- L125: `func execCommand(ctx context.Context, dir, name string, args []string) (string, int, error)`
- L148: `type capWriter struct`
- L153: `func (w *capWriter) Write(p []byte) (int, error)`
- L165: `func (w *capWriter) String() string { return w.buf.String() }`
- L173: `func capOutput(s string) string`
- L189: `func commandLine(name string, args []string) string`

### mock.go (245 LOC)
- L11: `type MockClient struct`
- L48: `func NewMockClient() *MockClient`
- L59: `func (m *MockClient) CreateSession(context.Context, SessionSpec) (string, error)`
- L70: `func (m *MockClient) Send(_ context.Context, _, prompt string, attachments []string, agentMode string) error`
- L84: `func (m *MockClient) SentModeAt(i int) string`
- L91: `func (m *MockClient) Abort(_ context.Context, sessionID string) error`
- L99: `type PermissionDecision struct`
- L105: `func (m *MockClient) Respond(id string, approve bool) error`
- L113: `type InputDecision struct`
- L119: `func (m *MockClient) RespondInput(id, answer string) error`
- L127: `type PlanDecision struct`
- L135: `func (m *MockClient) RespondPlan(id string, approved bool, action, feedback string) error`
- L143: `type ElicitDecision struct`
- L150: `func (m *MockClient) RespondElicit(id, action string, content map[string]any) error`
- L158: `func (m *MockClient) SentCount() int`
- L165: `func (m *MockClient) SentAt(i int) string`
- L172: `func (m *MockClient) ListModels(context.Context) ([]ModelInfo, error)`
- L179: `func (m *MockClient) AuthStatus(context.Context) (AuthStatus, error)`
- L186: `func (m *MockClient) ListSessions(context.Context) ([]SessionMeta, error)`
- L193: `func (m *MockClient) ResumeSession(_ context.Context, sessionID string, _ SessionSpec) (string, error)`
- L204: `func (m *MockClient) SessionHistory(_ context.Context, sessionID string) ([]Event, error)`
- L214: `func (m *MockClient) DeleteSession(_ context.Context, sessionID string) error`
- L225: `func (m *MockClient) Events() <-chan Event { return m.events }`
- L228: `func (m *MockClient) Emit(e Event)`
- L237: `func (m *MockClient) Close() error`

### normalize.go (494 LOC)
- L22: `type toolMeta struct`
- L28: `func postToolMeta(d *sdk.ToolExecutionStartData, argsSummary string) toolMeta`
- L43: `func toolKindFromName(name string, isMCP bool) string`
- L71: `func (c *SDKClient) makeHandler(sid string) func(sdk.SessionEvent)`
- L183: `func historyEvents(sid string, raw []sdk.SessionEvent) []Event`
- L213: `func normalizeUsage(d *sdk.AssistantUsageData) UsageData`
- L232: `func normalizeElicitFields(schema *sdk.ElicitationSchema) []ElicitField`
- L257: `func normalizeElicitField(name string, raw any) ElicitField`
- L277: `func elicitStr(m map[string]any, key string) string`
- L286: `func elicitStrSlice(v any) []string`
- L302: `func elicitDefault(v any) string`
- L323: `func sessionError(d *sdk.SessionErrorData) error`
- L335: `func planChangeText(op sdk.PlanChangedOperation) string`
- L350: `func compactionSummary(d *sdk.SessionCompactionCompleteData) string`
- L364: `func describePermission(req sdk.PermissionRequest) string`
- L379: `func permWriteFields(req sdk.PermissionRequest) (file, intention, diff string)`
- L389: `func summarizeArgs(args any) string`
- L411: `func stringField(m map[string]any, key string) (string, bool)`
- L424: `func toolResultText(d *sdk.ToolExecutionCompleteData) string`
- L441: `func oneLine(s string) string`
- L445: `func clip(s string, n int) string`
- L456: `func deref(p *int64) int64`
- L463: `func derefStr(p *string) string`
- L473: `func subagentSummary(durationMs, totalTokens *int64) string`
- L485: `func humanTokenCount(n int64) string`

### sdkclient.go (535 LOC)
- L25: `type SDKClient struct`
- L62: `type Options struct`
- L80: `func ResolveAuth(token string) (githubToken string, useLoggedInUser *bool)`
- L94: `func ResolveAuthMethod(method, configured string, ghToken func() (string, error)) (githubToken string, useLoggedInUser *bool)`
- L110: `func authStatusFromSDK(r *sdk.GetAuthStatusResponse) AuthStatus`
- L130: `func (c *SDKClient) AuthStatus(ctx context.Context) (AuthStatus, error)`
- L140: `func NewSDKClient(ctx context.Context, opts Options) (*SDKClient, error)`
- L184: `type sessionPolicy struct`
- L201: `func (c *SDKClient) applyHandlers() (onPerm sdk.PermissionHandlerFunc, onInput sdk.UserInputHandler, onPlan sdk.ExitPlanModeRequestHandler, onElicit sdk.ElicitationHandler)`
- L206: `func (c *SDKClient) CreateSession(ctx context.Context, spec SessionSpec) (string, error)`
- L248: `func (c *SDKClient) ListSessions(ctx context.Context) ([]SessionMeta, error)`
- L269: `func (c *SDKClient) ResumeSession(ctx context.Context, sessionID string, spec SessionSpec) (string, error)`
- L296: `func (c *SDKClient) SessionHistory(ctx context.Context, sessionID string) ([]Event, error)`
- L312: `func (c *SDKClient) DeleteSession(ctx context.Context, sessionID string) error`
- L331: `func shouldDropReasoningEffort(effort string, supported []string, known bool) bool`
- L349: `func (c *SDKClient) modelReasoningEfforts(ctx context.Context, model string) (efforts []string, known bool)`
- L375: `func (c *SDKClient) register(session *sdk.Session, spec SessionSpec)`
- L390: `func (c *SDKClient) ListModels(ctx context.Context) ([]ModelInfo, error)`
- L403: `func (c *SDKClient) Respond(id string, approve bool) error`
- L411: `func (c *SDKClient) RespondInput(id, answer string) error`
- L419: `func (c *SDKClient) RespondPlan(id string, approved bool, action, feedback string) error`
- L427: `func (c *SDKClient) RespondElicit(id, action string, content map[string]any) error`
- L435: `func (c *SDKClient) Send(ctx context.Context, sessionID, prompt string, attachments []string, agentMode string) error`
- L462: `func toAgentMode(mode string) sdk.AgentMode`
- L478: `func (c *SDKClient) Abort(ctx context.Context, sessionID string) error`
- L489: `func (c *SDKClient) Events() <-chan Event { return c.events }`
- L492: `func (c *SDKClient) emit(e Event)`
- L500: `func (c *SDKClient) Close() error`

## internal/ctxforge

### forge.go (593 LOC)
- L13: `type Forge struct`
- L30: `func New(dir string) *Forge`
- L36: `func Load(dir string) (*Forge, error)`
- L57: `func (f *Forge) Save() error`
- L80: `func (f *Forge) Validate() error`
- L150: `func uniqueIDs(kind string, n int, id func(int) string) error`
- L162: `func (f *Forge) Skill(id string) *Skill`
- L172: `func (f *Forge) Agent(id string) *Agent`
- L190: `func (f *Forge) HasOwnChatAgent() bool`
- L201: `type forgeState struct`
- L211: `func (f *Forge) snapshot() forgeState`
- L223: `func (f *Forge) restore(s forgeState)`
- L236: `func (f *Forge) mutate(apply func() error) error`
- L250: `func (f *Forge) AddSkill(s Skill) error`
- L264: `func (f *Forge) Instruction(id string) *Instruction`
- L274: `func (f *Forge) AddInstruction(in Instruction) error`
- L289: `func (f *Forge) AddAgent(a Agent) error`
- L305: `func (f *Forge) UpdateSkill(id string, s Skill) error`
- L318: `func (f *Forge) UpdateInstruction(id string, in Instruction) error`
- L331: `func (f *Forge) UpdateAgent(id string, a Agent) error`
- L343: `func (f *Forge) ToggleSkill(id string) (bool, error)`
- L353: `func (f *Forge) ToggleInstruction(id string) (bool, error)`
- L366: `func (f *Forge) RemoveSkill(id string) error`
- L379: `func (f *Forge) RemoveInstruction(id string) error`
- L394: `func (f *Forge) RemoveAgent(id string) error`
- L407: `func (f *Forge) MCPServer(id string) *MCPServer`
- L417: `func (f *Forge) AddMCPServer(m MCPServer) error`
- L433: `func (f *Forge) UpdateMCPServer(id string, m MCPServer) error`
- L445: `func (f *Forge) ToggleMCPServer(id string) (bool, error)`
- L456: `func (f *Forge) RemoveMCPServer(id string) error`
- L469: `type SessionSpec struct`
- L492: `func (f *Forge) Compile(agentID string) (SessionSpec, error)`
- L573: `func (f *Forge) activeSkills(agent *Agent) []Skill`

### hook.go (634 LOC)
- L58: `func EffectiveAutoApprove(mode string, configDefault bool) bool`
- L88: `type HookMatch struct`
- L99: `type Hook struct`
- L137: `func (h Hook) HasCommand() bool { return strings.TrimSpace(h.Command) != "" }`
- L146: `func hasDanglingVarRef(s string) bool`
- L163: `func (h Hook) Validate() error`
- L199: `func (h Hook) validateCommand() error`
- L223: `func (h Hook) appliesInMode(mode string) bool`
- L240: `func (m HookMatch) matches(toolKind, command, workspace string) bool`
- L260: `func isOutsideWorkspace(target, workspace string) bool`
- L292: `func PatternIsGlob(pattern string) bool { return strings.ContainsAny(pattern, "*?") }`
- L298: `func MatchPattern(pattern, command string) bool { return patternMatch(pattern, command) }`
- L302: `func patternMatch(pattern, command string) bool`
- L315: `func globMatch(pattern, s string) bool`
- L341: `type Decision struct`
- L372: `func Evaluate(hooks []Hook, event, toolKind, command, workspace, mode string) Decision`
- L429: `func PostToolUseCommands(hooks []Hook, toolKind, command, workspace, mode string) []Hook`
- L445: `func (f *Forge) Hook(id string) *Hook`
- L460: `func reservedHookID(id string) error`
- L468: `func (f *Forge) AddHook(h Hook) error`
- L487: `func (f *Forge) UpdateHook(id string, h Hook) error`
- L502: `func (f *Forge) ToggleHook(id string) (bool, error)`
- L513: `func (f *Forge) RemoveHook(id string) error`
- L531: `func DefaultHooks() []Hook`
- L564: `func DangerousHooks() []Hook`

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

### breakdown.go (73 LOC)
- L22: `type ModelBreakdown struct`
- L36: `func (b ModelBreakdown) USD() float64 { return b.USDTotal }`
- L39: `func (b ModelBreakdown) Credits() float64 { return b.USDTotal / USDPerCredit }`
- L46: `func ModelBreakdowns(records []SpendRecord) []ModelBreakdown`

### bucketforecast.go (89 LOC)
- L28: `type BucketProjection struct`
- L39: `func DailyTotalsBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) map[string][]DayTotal`
- L68: `func BucketForecasts(records []SpendRecord, budget Budget, now time.Time, keyOf func(SpendRecord) string, includeEmpty bool) []BucketProjection`

### credits.go (324 LOC)
- L11: `type Usage struct`
- L28: `func (u Usage) TotalTokens() int64`
- L33: `type Cost struct`
- L47: `func (c Cost) USD() float64 { return c.InputUSD + c.CachedUSD + c.OutputUSD + c.CacheWriteUSD }`
- L50: `func (c Cost) Credits() float64 { return c.USD() / USDPerCredit }`
- L54: `func Price(pb *PriceBook, u Usage) Cost`
- L75: `func EstimateTurn(pb *PriceBook, model string, contextTokens int64) Cost`
- L85: `type Meter struct`
- L101: `func (m *Meter) RecordReportedAIU(aiu float64)`
- L112: `func (m *Meter) ReportedAIU() float64`
- L121: `func (m *Meter) HasReported() bool { return HasReported(m.ReportedAIU()) }`
- L128: `func (m *Meter) ActualCredits() float64`
- L133: `type ModelTotals struct`
- L148: `func (m ModelTotals) USD() float64 { return m.InputUSD + m.CachedUSD + m.OutputUSD + m.CacheWriteUSD }`
- L151: `func (m ModelTotals) Credits() float64 { return m.USD() / USDPerCredit }`
- L155: `func NewMeter(pb *PriceBook) *Meter`
- L165: `func (m *Meter) PriceBook() *PriceBook { return m.pb }`
- L169: `func (m *Meter) Record(u Usage) Cost`
- L210: `func (m *Meter) ExtraTokens() (cacheWrite, reasoning int64)`
- L219: `func (m *Meter) EstimateTurn(model string, contextTokens int64) Cost`
- L224: `func (m *Meter) Totals() Cost`
- L234: `func (m *Meter) TotalTokens() (input, cached, output int64)`
- L247: `func (m *Meter) ByModel() []ModelTotals`
- L264: `func (m *Meter) Count() int`
- L273: `type Budget struct`
- L285: `func (b Budget) Remaining(used float64) float64 { return b.AllowanceCredits - used }`
- L289: `func (b Budget) FractionUsed(used float64) float64`
- L302: `func (b Budget) Warned(used float64) bool`
- L312: `func (b Budget) CapExceeded(projected float64) bool`
- L321: `func FormatUSD(v float64) string { return fmt.Sprintf("$%.4f", v) }`
- L324: `func FormatCredits(v float64) string { return fmt.Sprintf("%.2f cr", v) }`

### dashboard.go (157 LOC)
- L18: `type DayPoint struct`
- L29: `type WindowSpend struct`
- L37: `func (w WindowSpend) AvgCostPerTurn() float64`
- L46: `func (w WindowSpend) DailyRate() float64`
- L58: `type WindowDashboard struct`
- L71: `func Dashboard(records []SpendRecord, now time.Time, window int) WindowDashboard`
- L125: `type Delta struct`
- L139: `func ChangePct(prior, current float64) Delta`

### drift.go (84 LOC)
- L25: `type ModelDrift struct`
- L38: `func (d ModelDrift) Drifted(epsilon float64) bool { return math.Abs(d.Delta) >= epsilon }`
- L49: `func ModelDrifts(records []SpendRecord) []ModelDrift`

### forecast.go (142 LOC)
- L24: `type ProjectionStatus int`
- L45: `type Projection struct`
- L76: `func Forecast(daily []DayTotal, budget Budget, now time.Time) Projection`

### history.go (361 LOC)
- L24: `type SpendRecord struct`
- L61: `func (r SpendRecord) Credits() float64 { return r.USD / USDPerCredit }`
- L65: `func (r SpendRecord) EstimateCredits() float64 { return r.Credits() }`
- L69: `func (r SpendRecord) HasReported() bool { return HasReported(r.AIU) }`
- L74: `func (r SpendRecord) ActualCredits() float64 { return ActualCredits(r.EstimateCredits(), r.AIU) }`
- L77: `func (r SpendRecord) Day() string { return r.At.UTC().Format("2006-01-02") }`
- L97: `type SpendStore struct`
- L107: `func LoadSpendStore(dir string) (*SpendStore, error)`
- L117: `func stampSpend(r SpendRecord) SpendRecord`
- L125: `type DayTotal struct`
- L134: `func DailyTotals(records []SpendRecord) []DayTotal`
- L172: `func MonthToDate(records []SpendRecord, now time.Time) Cost`
- L186: `type share struct`
- L200: `func shareBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) []share`
- L231: `type ModelShare struct`
- L241: `func ModelShares(records []SpendRecord) []ModelShare`
- L253: `type AgentShare struct`
- L264: `func AgentShares(records []SpendRecord) []AgentShare`
- L275: `type WorkflowShare struct`
- L287: `func WorkflowShares(records []SpendRecord) []WorkflowShare`
- L298: `type SessionShare struct`
- L311: `func SessionShares(records []SpendRecord) []SessionShare`
- L323: `func WriteCSV(w io.Writer, records []SpendRecord) error`
- L359: `func csvFloat(v float64) string`

### pricing.go (226 LOC)
- L27: `type ModelRate struct`
- L56: `func withDerivedCacheWrite(r ModelRate) ModelRate`
- L73: `type PriceBook struct`
- L82: `func NewPriceBook(fallback ModelRate, rates ...ModelRate) *PriceBook`
- L93: `func DefaultPriceBook() *PriceBook`
- L110: `func (pb *PriceBook) Rate(model string) (ModelRate, bool)`
- L125: `func (pb *PriceBook) Set(r ModelRate)`
- L135: `func (pb *PriceBook) Models() []string`
- L156: `func (pb *PriceBook) Replace(src *PriceBook)`
- L188: `func BuildPriceBook(overrides map[string][]float64) *PriceBook`
- L215: `func normalizeModel(m string) string`
- L223: `func (r ModelRate) String() string`

### reconcile.go (237 LOC)
- L32: `type WorkflowRecon struct`
- L52: `func WorkflowReconcile(spend []SpendRecord, runs []RunRecord) []WorkflowRecon`
- L114: `type LaneRecon struct`
- L138: `func LaneReconcile(spend []SpendRecord, runs []RunRecord) []LaneRecon`
- L218: `func WriteReconcileCSV(w io.Writer, spend []SpendRecord, runs []RunRecord) error`

### runs.go (333 LOC)
- L25: `type RunLane struct`
- L36: `type RunRecord struct`
- L51: `func (r RunRecord) Credits() float64`
- L63: `func (r RunRecord) Duration() time.Duration`
- L88: `type RunStore struct`
- L96: `func LoadRunStore(dir string) (*RunStore, error)`
- L106: `func stampRun(r RunRecord) RunRecord`
- L120: `type RunAggregate struct`
- L143: `func (a RunAggregate) FailureRate() float64`
- L159: `func RunAggregates(records []RunRecord) []RunAggregate`
- L214: `type LaneShare struct`
- L232: `func LaneShares(records []RunRecord) []LaneShare`
- L287: `func WriteRunsCSV(w io.Writer, records []RunRecord) error`
- L317: `func csvTime(t time.Time) string`
- L328: `func laterRun(s2, f2, s1, f1 time.Time) bool`

### spend_source.go (91 LOC)
- L22: `func HasReported(reportedAIU float64) bool { return reportedAIU > 0 }`
- L27: `func ReportedCredits(reportedAIU float64) float64 { return reportedAIU }`
- L32: `func ActualCredits(estimateCredits, reportedAIU float64) float64`
- L44: `type ActualSpend struct`
- L67: `func MonthToDateActual(records []SpendRecord, now time.Time) ActualSpend`

### store.go (185 LOC)
- L41: `type AppendOnlyStore[T any] struct`
- L57: `func loadAppendOnlyStore[T any](dir, file, key, what string, version int, stamp func(T) T) (*AppendOnlyStore[T], error)`
- L82: `func decodeEnvelope[T any](data []byte, key string) ([]T, error)`
- L102: `func (s *AppendOnlyStore[T]) Append(r T) error`
- L114: `func (s *AppendOnlyStore[T]) save() error`
- L134: `func (s *AppendOnlyStore[T]) Records() []T`
- L143: `func (s *AppendOnlyStore[T]) Count() int`
- L155: `type envelope[T any] struct`
- L161: `func (e envelope[T]) MarshalJSON() ([]byte, error)`
- L183: `func encodeEnvelope[T any](version int, key string, records []T) ([]byte, error)`

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

### budget.go (118 LOC)
- L17: `type budgetTracker struct`
- L25: `func (b budgetTracker) Budget() telemetry.Budget`
- L38: `func (b budgetTracker) MonthToDate(now time.Time) telemetry.Cost`
- L50: `func (b budgetTracker) MonthToDateActual(now time.Time) telemetry.ActualSpend`
- L62: `func (b budgetTracker) Forecast(now time.Time) (telemetry.Projection, bool)`
- L76: `func (s *Server) budgets() budgetTracker`
- L88: `func (s *Server) budget() telemetry.Budget { return s.budgets().Budget() }`
- L91: `func (s *Server) monthToDate() telemetry.Cost { return s.budgets().MonthToDate(time.Now()) }`
- L95: `func (s *Server) monthToDateActual() telemetry.ActualSpend`
- L100: `func (s *Server) forecast(now time.Time) (telemetry.Projection, bool)`
- L110: `func (s *Server) overCap() (projected float64, capped bool)`

### commands.go (424 LOC)
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
- L310: `func SeamSpec(cs ctxforge.SessionSpec, defModel, defEffort string, lookupEnv func(string) string, workspace string, autoApprove bool) copilot.SessionSpec`
- L328: `func (s *Server) compiledSpec(agentID string) copilot.SessionSpec`
- L342: `func (s *Server) applyAgentSpec(c copilot.SessionSpec, agentID string) string`
- L359: `func (s *Server) cmdCost() string`
- L380: `func (s *Server) cmdAttach(path string) string`
- L394: `func (s *Server) cmdNav(slug string) string`
- L399: `func isNavSlug(slug string) bool`
- L410: `func commandHelp() string`

### connection.go (145 LOC)
- L22: `type authRung struct`
- L32: `func precedenceRows(method, tokenEnv string) []authRung`
- L51: `func (s *Server) connectionPartial() string { return s.renderConnection("", "") }`
- L56: `func (s *Server) renderConnection(note, errMsg string) string`
- L100: `func (s *Server) handleConnectionSave(w http.ResponseWriter, r *http.Request)`

### dashboard_render.go (153 LOC)
- L25: `func (s *Server) dashboardView(window int, now time.Time) map[string]any`
- L113: `func kpiCard(label, value string, series []float64, delta telemetry.Delta, higherIsWorse bool, window int) map[string]any`
- L128: `func deltaView(d telemetry.Delta, higherIsWorse bool) map[string]any`

### demo.go (152 LOC)
- L16: `func streamDemoReply(m *copilot.MockClient, prompt string)`
- L143: `func tokenize(s string) []string`

### diff.go (157 LOC)
- L15: `type diffLineKind int`
- L30: `type diffLine struct`
- L41: `type diffView struct`
- L53: `func parseUnifiedDiff(s string) diffView`
- L107: `func isHunkHeader(s string) bool`
- L117: `func hunkStarts(s string) (oldStart, newStart int)`
- L134: `func leadingInt(s string) int`
- L147: `func isFileHeader(s string) bool`

### entities.go (95 LOC)
- L11: `func (s *Server) skillsPartial() string`
- L23: `func (s *Server) instructionsPartial() string`
- L36: `func (s *Server) agentsPartial() string`
- L63: `func (s *Server) modelsPartial() string`
- L93: `func (s *Server) settingsPartial() string`

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

### help.go (123 LOC)
- L14: `func renderShortcuts(keymap []config.ResolvedKey) string`
- L28: `func helpOverlay(keymap []config.ResolvedKey) string { return helpOverlayAttr(keymap, "") }`
- L34: `func helpOverlayAttr(keymap []config.ResolvedKey, extraAttr string) string`
- L46: `func keymapJSON(keymap []config.ResolvedKey) string`
- L63: `func keymapLiveApply(keymap []config.ResolvedKey) string`
- L71: `func (s *Server) helpPartial() string`

### hooks.go (331 LOC)
- L47: `func (s *Server) hooksPartial() string`
- L69: `func hookRow(h ctxforge.Hook, builtin bool) map[string]any`
- L80: `func hookSummary(h ctxforge.Hook) string`
- L97: `func shortEvent(ev string) string`
- L114: `func renderHookForm(h ctxforge.Hook, isNew bool, errMsg string) string`
- L142: `func modesField(selected []string) string`
- L162: `func parseModes(r *http.Request) []string`
- L178: `func hookFromForm(r *http.Request, id string) ctxforge.Hook`
- L215: `func (s *Server) handleHookNew(w http.ResponseWriter, r *http.Request)  { hookCRUD.New(s, w, r) }`
- L216: `func (s *Server) handleHookEdit(w http.ResponseWriter, r *http.Request) { hookCRUD.Edit(s, w, r) }`
- L226: `func (s *Server) handleHookCreate(w http.ResponseWriter, r *http.Request)`
- L235: `func (s *Server) handleHookUpdate(w http.ResponseWriter, r *http.Request) { hookCRUD.Update(s, w, r) }`
- L237: `func (s *Server) handleHookToggle(w http.ResponseWriter, r *http.Request)`
- L243: `func (s *Server) handleHookDelete(w http.ResponseWriter, r *http.Request)`
- L253: `func (s *Server) handleHookPreflight(w http.ResponseWriter, r *http.Request)`
- L263: `func (s *Server) handleHookCommandPreflight(w http.ResponseWriter, r *http.Request)`
- L277: `func hookCommandPreflightResult(command string, args []string, lookup func(string) string) string`
- L303: `func commandVarRefs(s string) []string`
- L317: `func hookPreflightResult(pattern, sample string) string`

### hub.go (312 LOC)
- L28: `type Hub struct`
- L66: `type Options struct`
- L88: `func New(opts Options) *Hub`
- L118: `func (h *Hub) newSession(id string) *Server`
- L148: `func (h *Hub) session(w http.ResponseWriter, r *http.Request) *Server`
- L177: `func (h *Hub) bind(copilotID string, s *Server)`
- L186: `func (h *Hub) route(copilotID string) *Server`
- L202: `func (h *Hub) pump()`
- L212: `func (h *Hub) Handler() http.Handler`
- L305: `func (s *Server) Handler() http.Handler { return s.hub.Handler() }`
- L308: `func newID() string`

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

### pages.go (199 LOC)
- L53: `func clampWindow(raw string) int`
- L70: `func (s *Server) renderPage(slug, window string) string`
- L109: `func (s *Server) chatPartial() string`
- L144: `func (s *Server) agentLabel(id string) string`
- L156: `func (s *Server) workflowLabel(id string) string`
- L167: `func (s *Server) handleRunsExport(w http.ResponseWriter, r *http.Request)`
- L180: `func addData(kind, noun string) map[string]any { return map[string]any{"Kind": kind, "Noun": noun} }`
- L182: `func def(s, fallback string) string`
- L190: `func truncate(s string, n int) string`

### palette.go (33 LOC)
- L15: `func commandPalette() string`

### render.go (561 LOC)
- L25: `func esc(s string) string`
- L31: `func deltaSpan(text string) string { return frag("deltaSpan", text) }`
- L34: `func renderTurn(t convo.Turn) string`
- L60: `func renderHookRunNote(h *convo.HookRunView) string`
- L86: `func renderDecisionNote(d *convo.DecisionView) string`
- L109: `func renderToolCard(tv *convo.ToolView) string`
- L133: `func renderCur(role convo.Role, text string) string`
- L143: `func renderTimelineInner(st *convo.State) string`
- L166: `func renderPermForm(req copilot.PermissionRequest) string`
- L185: `func diffLineViews(lines []diffLine) []map[string]any`
- L208: `func diffClass(k diffLineKind) string`
- L225: `func diffMarker(k diffLineKind) string`
- L241: `func diffLabel(k diffLineKind) string`
- L253: `func gutterNum(n int) string`
- L263: `func renderAskForm(req copilot.InputRequest) string`
- L273: `func renderPlanForm(req copilot.PlanRequest) string`
- L283: `func renderElicitForm(req copilot.ElicitRequest) string`
- L295: `func elicitFieldView(f copilot.ElicitField) map[string]any`
- L320: `func elicitFieldKey(name string) string { return "f." + name }`
- L324: `func subagentLabel(sa copilot.SubagentInfo) string`
- L339: `func renderSubagents(active []copilot.SubagentInfo) string`
- L360: `func renderStatus(text string, active bool, startMs int64) string`
- L368: `func renderCtx(cur, limit int64, compacting bool) string`
- L403: `func renderStatline(s *Server) string`
- L464: `func statlineForecast(s *Server, now time.Time) (show bool, short string, warn bool, title string)`
- L478: `func humanTokens(n int64) string`
- L494: `func renderCostFooter(credits float64, budget telemetry.Budget) string`
- L514: `func renderActualCostFooter(a telemetry.ActualSpend, budget telemetry.Budget) string`
- L546: `func renderBudgetForm(projected, capCredits float64) string`
- L555: `func clampLines(s string, n int) string`

### runs.go (195 LOC)
- L28: `func (s *Server) runsPartial(window int) string`
- L62: `func windowRuns(records []telemetry.RunRecord, window int) []telemetry.RunRecord`
- L91: `func (s *Server) runSummaryRow(a telemetry.RunAggregate) map[string]any`
- L110: `func (s *Server) laneShareRow(l telemetry.LaneShare) map[string]any`
- L132: `func (s *Server) runRow(r telemetry.RunRecord, window int) map[string]any`
- L162: `func humanDuration(d time.Duration) string`
- L190: `func runOutcomeGlyph(outcome string) (glyph, state string)`

### server.go (863 LOC)
- L27: `type Server struct`
- L98: `func (s *Server) subscribe() chan fragment`
- L107: `func (s *Server) unsubscribe(ch chan fragment)`
- L118: `func (s *Server) broadcast(frags []fragment)`
- L137: `func (s *Server) broadcastSendFailure(err error)`
- L142: `type indexData struct`
- L153: `type navItem struct`
- L161: `type navGroup struct`
- L171: `func navGroups() []navGroup`
- L189: `func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request)`
- L208: `func (s *Server) ensureSession(ctx context.Context) (string, error)`
- L227: `func (s *Server) handleSend(w http.ResponseWriter, r *http.Request)`
- L324: `type budgetGate struct`
- L334: `func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request)`
- L390: `func (s *Server) dispatch(ctx context.Context, sessionID, prompt string, attachments []string) error`
- L409: `func (s *Server) sendFailedOOB(err error) string`
- L421: `func queuedStatus(n int) string`
- L428: `func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request)`
- L449: `func (s *Server) handlePerm(w http.ResponseWriter, r *http.Request)`
- L479: `func (s *Server) dropLanePerm(id string) string`
- L496: `func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request)`
- L516: `func (s *Server) handlePlanReview(w http.ResponseWriter, r *http.Request)`
- L549: `func (s *Server) handleElicit(w http.ResponseWriter, r *http.Request)`
- L587: `func elicitContent(fields []copilot.ElicitField, form url.Values) map[string]any`
- L617: `func (s *Server) handlePage(w http.ResponseWriter, r *http.Request)`
- L630: `func (s *Server) handleSkillToggle(w http.ResponseWriter, r *http.Request)`
- L636: `func (s *Server) handleSkillDelete(w http.ResponseWriter, r *http.Request)`
- L640: `func (s *Server) handleInstructionToggle(w http.ResponseWriter, r *http.Request)`
- L646: `func (s *Server) handleInstructionDelete(w http.ResponseWriter, r *http.Request)`
- L650: `func (s *Server) handleAgentSelect(w http.ResponseWriter, r *http.Request)`
- L674: `func (s *Server) handleModelSelect(w http.ResponseWriter, r *http.Request)`
- L681: `func (s *Server) handleEffortSelect(w http.ResponseWriter, r *http.Request)`
- L690: `func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request)`
- L713: `func (s *Server) oobTimeline() string`
- L718: `func oobStatus(text string, active bool, startMs int64) string`
- L724: `func (s *Server) oobBudget() string`
- L730: `func (s *Server) budgetFrag() fragment`
- L736: `func (s *Server) renderGate() string`
- L745: `func (s *Server) statusFrag(text string, active bool) fragment`
- L755: `func (s *Server) ctxFrag() fragment`
- L761: `func (s *Server) statFrag() fragment`
- L768: `func (s *Server) oobStat() string`
- L774: `func (s *Server) subagentsFrag() fragment`
- L779: `func nowMs() int64 { return time.Now().UnixMilli() }`
- L784: `func dropByID[T any](sl []T, id string, key func(T) string) []T`
- L795: `func findByID[T any](sl []T, id string, key func(T) string) (T, bool)`
- L805: `func permID(p copilot.PermissionRequest) string { return p.ID }`
- L806: `func inputID(p copilot.InputRequest) string     { return p.ID }`
- L807: `func planID(p copilot.PlanRequest) string       { return p.ID }`
- L808: `func elicitID(e copilot.ElicitRequest) string   { return e.ID }`
- L809: `func subagentKey(a copilot.SubagentInfo) string { return a.ToolCallID }`
- L814: `func (s *Server) dropPerm(id string)  { s.perms = dropByID(s.perms, id, permID) }`
- L815: `func (s *Server) dropInput(id string) { s.inputs = dropByID(s.inputs, id, inputID) }`
- L816: `func (s *Server) dropPlan(id string)  { s.plans = dropByID(s.plans, id, planID) }`
- L817: `func (s *Server) dropElicit(id string)`
- L823: `func (s *Server) findElicit(id string) (copilot.ElicitRequest, bool)`
- L828: `func (s *Server) dropSubagent(toolCallID string)`
- L833: `func firstNonEmpty(vals []string) string`
- L846: `func (s *Server) editForge(fn func() error) error`
- L860: `func (s *Server) writePartial(w http.ResponseWriter, html string)`

### session.go (365 LOC)
- L15: `type liveKind int`
- L28: `func (s *Server) handleEvent(e copilot.Event) []fragment`
- L290: `type spendTag struct`
- L306: `func (s *Server) recordUsage(u copilot.UsageData, tag spendTag) telemetry.Cost`
- L347: `func (s *Server) timelineFragments() []fragment`
- L360: `func toolID(e copilot.Event) string`

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

### settings.go (335 LOC)
- L26: `func (s *Server) editConfig(fn func(*config.Config)) error`
- L41: `func (s *Server) refreshBudget()`
- L55: `func (s *Server) renderSettings(note, errMsg string) string`
- L62: `func renderSettingsForm(c *config.Config, note, errMsg string) string`
- L105: `func priceOverrideFields(c *config.Config) []string`
- L141: `func priceRowField(i int, model string, ov []float64, has bool, def telemetry.ModelRate) string`
- L168: `func parsePriceOverrides(r *http.Request) map[string][]float64`
- L209: `func rateOrDefault(s string, def float64) float64`
- L224: `func formHasPriceOverrides(r *http.Request) bool`
- L237: `func formHasKeyBindings(r *http.Request) bool`
- L248: `func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request)`

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

### svg.go (238 LOC)
- L35: `func svgNum(v float64) string`
- L41: `func scaleX(i, n int, w, pad float64) float64`
- L51: `func scaleY(v, max, h, pad float64) float64`
- L60: `func seriesMax(series []float64) float64`
- L73: `func sparkPoints(series []float64, w, h, pad float64) string`
- L91: `func areaPath(series []float64, w, h, pad float64) string`
- L114: `func bulletGeom(value, target, scaleMax, w, pad float64) (barW, targetX float64)`
- L136: `func svgOpen(class string, w, h float64, label string) string`
- L148: `func sparklineSVG(series []float64, label string) string`
- L163: `func trendBandSVG(actual, forecast []float64, label string) string`
- L216: `func bulletSVG(value, target, scaleMax float64, overTarget bool, label string) string`

### telemetry_render.go (426 LOC)
- L12: `func (s *Server) telemetryPartial(window int) string`
- L83: `func (s *Server) spendTrend(window int) (days, shares []map[string]any, hasHistory bool)`
- L140: `func (s *Server) spendShares(now time.Time) (agents, workflows []map[string]any)`
- L170: `func agentKey(r telemetry.SpendRecord) string { return r.AgentID }`
- L172: `func workflowKey(r telemetry.SpendRecord) string { return r.WorkflowID }`
- L190: `func (s *Server) workflowReconcile() []map[string]any`
- L211: `func (s *Server) reconcileRow(r telemetry.WorkflowRecon) map[string]any`
- L228: `func (s *Server) laneReconcile() []map[string]any`
- L250: `func (s *Server) laneReconcileRow(r telemetry.LaneRecon) map[string]any`
- L267: `func (s *Server) estimateDrift() []map[string]any`
- L293: `func bucketTrajectories(bs []telemetry.BucketProjection, now time.Time) map[string]string`
- L309: `func bucketTrajectoryText(p telemetry.Projection, now time.Time) string`
- L328: `func daysLeftInMonth(now time.Time) int`
- L339: `func shareRow(label string, credits, fraction float64) map[string]any`
- L351: `func forecastView(fc telemetry.Projection, allowance float64, now time.Time) map[string]any`
- L375: `func forecastSoon(exhaust, now time.Time) bool`
- L385: `func plural(n int, one, many string) string`
- L395: `func (s *Server) handleSpendExport(w http.ResponseWriter, r *http.Request)`
- L412: `func (s *Server) handleReconcileExport(w http.ResponseWriter, r *http.Request)`

### tmpl.go (55 LOC)
- L44: `func trusted(s string) template.HTML { return template.HTML(s) } //nolint:gosec // composed from escaped fragments`
- L49: `func frag(name string, data any) string`

### workflow.go (1123 LOC)
- L29: `type laneStatus int`
- L42: `func settled(st laneStatus) bool`
- L48: `type lane struct`
- L68: `func (l *lane) toolStart(id, name, args string)`
- L82: `func (l *lane) toolProgress(id, msg string)`
- L89: `func (l *lane) toolEnd(id, result string, success bool)`
- L100: `func (l *lane) toolByID(id string) *convo.ToolView`
- L112: `func (l *lane) dropPerm(id string) bool`
- L126: `type workflowRun struct`
- L145: `func newWorkflowRun(wf ctxforge.Workflow, steps []ctxforge.CompiledStep, specs []copilot.SessionSpec) *workflowRun`
- L160: `func (r *workflowRun) start() []int`
- L179: `func (r *workflowRun) evalWhen(l *lane) (satisfied, ready bool)`
- L200: `func skipDetail(w *ctxforge.StepCondition) string`
- L219: `func (r *workflowRun) evalPending() []int`
- L250: `func (r *workflowRun) handoffPrompt(idx int) string`
- L270: `func (r *workflowRun) laneFor(sessionID string) *lane`
- L291: `func (l *lane) appendText(s string) { l.text += s }`
- L295: `func (r *workflowRun) finishLane(l *lane, detail string) []int`
- L307: `func (r *workflowRun) failLane(l *lane, msg string) []int`
- L323: `func (r *workflowRun) abort() []string`
- L344: `func (r *workflowRun) advance(l *lane) []int`
- L369: `func (r *workflowRun) allSettled() bool`
- L384: `func (s *Server) handleWorkflowRun(w http.ResponseWriter, r *http.Request)`
- L404: `func (s *Server) handleRunRerun(w http.ResponseWriter, r *http.Request)`
- L419: `func (s *Server) handleRunAbort(w http.ResponseWriter, r *http.Request)`
- L432: `func (s *Server) abortRun(ctx context.Context)`
- L459: `func (s *Server) launchWorkflow(id string) bool`
- L498: `func (s *Server) workflowLaneSpec(cs ctxforge.SessionSpec) copilot.SessionSpec`
- L509: `func (s *Server) launchLanes(run *workflowRun, idxs []int)`
- L517: `func (s *Server) startLane(run *workflowRun, idx int)`
- L547: `func (s *Server) laneError(run *workflowRun, l *lane, err error)`
- L561: `func (s *Server) handleRunEvent(run *workflowRun, e copilot.Event) []fragment`
- L661: `func (s *Server) runFrags(run *workflowRun, done bool) []fragment`
- L694: `func (s *Server) recordRun(run *workflowRun)`
- L707: `func runRecord(run *workflowRun) telemetry.RunRecord`
- L727: `func laneStatusName(st laneStatus) string`
- L743: `func (l *lane) costDetail() string`
- L751: `func (s *Server) lanesFrag() fragment`
- L758: `func renderLanes(run *workflowRun) string`
- L782: `func laneToolsHTML(tools []*convo.ToolView) template.HTML`
- L793: `func lanePermsHTML(perms []copilot.PermissionRequest) template.HTML`
- L804: `func laneGlyph(st laneStatus) (glyph, state string)`
- L811: `func glyphFor(status string) (glyph, state string)`
- L834: `func streamDemoLane(m *copilot.MockClient, sid, prompt string)`
- L876: `func firstLine(s string) string`
- L887: `func (s *Server) workflowsPartial() string`
- L943: `func renderWorkflowForm(w ctxforge.Workflow, isNew bool, errMsg string) string`
- L964: `func stepsToText(steps []ctxforge.WorkflowStep) string`
- L984: `func stepsFromText(raw string) []ctxforge.WorkflowStep`
- L1013: `func splitStepLine(line string) (head, prompt string)`
- L1032: `func formatStepCondition(c *ctxforge.StepCondition) string`
- L1050: `func parseStepCondition(spec string) *ctxforge.StepCondition`
- L1070: `func workflowFromForm(r *http.Request, id string) ctxforge.Workflow`
- L1080: `func (s *Server) handleWorkflowNew(w http.ResponseWriter, r *http.Request)`
- L1084: `func (s *Server) handleWorkflowEdit(w http.ResponseWriter, r *http.Request)`
- L1099: `func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request)`
- L1108: `func (s *Server) handleWorkflowUpdate(w http.ResponseWriter, r *http.Request)`
- L1118: `func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request)`

