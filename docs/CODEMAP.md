# CODEMAP — generated, do not edit by hand

> Regenerate with `make codemap`. A per-package index of every top-level
> `type`/`func` (with its file and line count) so a session can learn the
> layout from this one file instead of opening source to find a symbol. Read
> this first; jump straight to `file:symbol`. The source is the source of
> truth — if this looks stale, re-run `make codemap`.

_Last generated: 2026-06-11 (UTC)._

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

### bootstrap.go (424 LOC)
- L36: `func Build(configDir string, demo bool) (srv *web.Hub, close func(), err error)`
- L122: `func demoClient(forge *ctxforge.Forge, spec *copilot.SessionSpec) (copilot.Client, func())`
- L167: `func seedSpend(store *telemetry.SpendStore)`
- L197: `func seedRuns(store *telemetry.RunStore)`
- L225: `func dialClient(cfg *config.Config) (copilot.Client, func())`
- L248: `func ghCLIToken() (string, error)`
- L263: `func ServeLocal(h http.Handler) (port int, stop func(), err error)`
- L275: `func DefaultConfigDir() string`
- L289: `func SeedForge(forge *ctxforge.Forge)`
- L412: `func curatedMCPServers() []ctxforge.MCPServer`

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

### subagents.go (454 LOC)
- L23: `type SubagentStatus int`
- L34: `func (st SubagentStatus) Label() string`
- L48: `func (st SubagentStatus) Class() string`
- L64: `type SubagentEntryKind int`
- L77: `type SubagentEntry struct`
- L94: `type SubagentView struct`
- L118: `type Subagents struct`
- L128: `func (r *Subagents) Start(spawnID, name, displayName, description, model string)`
- L145: `func (r *Subagents) Observe(instanceID, activity string) bool`
- L160: `func (r *Subagents) AddCredits(instanceID string, credits float64) bool`
- L175: `func (r *Subagents) AppendText(instanceID string, reasoning bool, text string) bool`
- L205: `func (r *Subagents) CommitText(instanceID string, reasoning bool, text string) bool`
- L246: `func (r *Subagents) RecordTool(instanceID, toolID, name, args string) bool`
- L267: `func (r *Subagents) capTranscript(e *SubagentView)`
- L277: `func (r *Subagents) MarkInputRequired(id string) bool`
- L289: `func (r *Subagents) ClearInputRequired(id string) bool`
- L301: `func (r *Subagents) byAnyID(id string) *SubagentView`
- L310: `func (r *Subagents) byInstance(instanceID string) *SubagentView`
- L326: `func (r *Subagents) RecordSteer(spawnID, text string) bool`
- L341: `func (r *Subagents) SetLaneSession(spawnID, session string)`
- L350: `func (r *Subagents) ByID(spawnID string) (SubagentView, bool)`
- L361: `func (r *Subagents) ViewByInstance(instanceID string) (SubagentView, bool)`
- L370: `func copyView(e *SubagentView) SubagentView`
- L381: `func (r *Subagents) NameFor(instanceID string) string`
- L398: `func (r *Subagents) End(spawnID string, success bool, detail string, totalTokens int64) bool`
- L415: `func (r *Subagents) Entries() []SubagentView`
- L422: `func (r *Subagents) Empty() bool { return len(r.entries) == 0 }`
- L425: `func (r *Subagents) Reset() { r.entries = nil }`
- L428: `func (r *Subagents) bySpawn(spawnID string) *SubagentView`
- L440: `func (r *Subagents) join(instanceID string) *SubagentView`

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

### copilot.go (362 LOC)
- L17: `type EventType int`
- L48: `type SessionMeta struct`
- L59: `type PermissionRequest struct`
- L76: `type InputRequest struct`
- L91: `type SubagentInfo struct`
- L108: `type ElicitRequest struct`
- L119: `type ElicitField struct`
- L136: `type PlanRequest struct`
- L146: `type UsageData struct`
- L159: `type Event struct`
- L199: `type HookRun struct`
- L215: `type ToolDecision struct`
- L225: `type ContextInfo struct`
- L232: `type ModelInfo struct`
- L243: `type AuthStatus struct`
- L254: `type ToolCall struct`
- L271: `type SessionSpec struct`
- L297: `type MCPServer struct`
- L312: `func (m MCPServer) Key() string`
- L320: `type Client interface`

### handlers.go (197 LOC)
- L31: `func (c *SDKClient) permissionHandler() sdk.PermissionHandlerFunc`
- L99: `func isBuiltinHook(id string) bool { return strings.HasPrefix(id, "builtin-") }`
- L105: `func (c *SDKClient) emitDecision(sessionID, kind string, d ctxforge.Decision, req sdk.PermissionRequest)`
- L116: `func permCommand(req sdk.PermissionRequest) string`
- L130: `func (c *SDKClient) userInputHandler() sdk.UserInputHandler`
- L157: `func (c *SDKClient) exitPlanModeHandler() sdk.ExitPlanModeRequestHandler`
- L180: `func (c *SDKClient) elicitationHandler() sdk.ElicitationHandler`

### hookexec.go (198 LOC)
- L40: `type commandRunner func(ctx context.Context, dir, name string, args []string) (output string, exitCode int, err error)`
- L50: `func (c *SDKClient) resolveVarRefs(s string) string`
- L62: `func (c *SDKClient) hookTimeoutOrDefault() time.Duration`
- L77: `func (c *SDKClient) firePostToolHooks(sid, agentID string, hooks []ctxforge.Hook, workspace string)`
- L90: `func (c *SDKClient) runPostToolHook(h ctxforge.Hook, workspace string) Event`
- L129: `func execCommand(ctx context.Context, dir, name string, args []string) (string, int, error)`
- L152: `type capWriter struct`
- L157: `func (w *capWriter) Write(p []byte) (int, error)`
- L169: `func (w *capWriter) String() string { return w.buf.String() }`
- L177: `func capOutput(s string) string`
- L193: `func commandLine(name string, args []string) string`

### mock.go (247 LOC)
- L11: `type MockClient struct`
- L49: `func NewMockClient() *MockClient`
- L60: `func (m *MockClient) CreateSession(context.Context, SessionSpec) (string, error)`
- L71: `func (m *MockClient) Send(_ context.Context, sessionID, prompt string, attachments []string, agentMode string) error`
- L86: `func (m *MockClient) SentModeAt(i int) string`
- L93: `func (m *MockClient) Abort(_ context.Context, sessionID string) error`
- L101: `type PermissionDecision struct`
- L107: `func (m *MockClient) Respond(id string, approve bool) error`
- L115: `type InputDecision struct`
- L121: `func (m *MockClient) RespondInput(id, answer string) error`
- L129: `type PlanDecision struct`
- L137: `func (m *MockClient) RespondPlan(id string, approved bool, action, feedback string) error`
- L145: `type ElicitDecision struct`
- L152: `func (m *MockClient) RespondElicit(id, action string, content map[string]any) error`
- L160: `func (m *MockClient) SentCount() int`
- L167: `func (m *MockClient) SentAt(i int) string`
- L174: `func (m *MockClient) ListModels(context.Context) ([]ModelInfo, error)`
- L181: `func (m *MockClient) AuthStatus(context.Context) (AuthStatus, error)`
- L188: `func (m *MockClient) ListSessions(context.Context) ([]SessionMeta, error)`
- L195: `func (m *MockClient) ResumeSession(_ context.Context, sessionID string, _ SessionSpec) (string, error)`
- L206: `func (m *MockClient) SessionHistory(_ context.Context, sessionID string) ([]Event, error)`
- L216: `func (m *MockClient) DeleteSession(_ context.Context, sessionID string) error`
- L227: `func (m *MockClient) Events() <-chan Event { return m.events }`
- L230: `func (m *MockClient) Emit(e Event)`
- L239: `func (m *MockClient) Close() error`

### normalize.go (495 LOC)
- L22: `type toolMeta struct`
- L28: `func postToolMeta(d *sdk.ToolExecutionStartData, argsSummary string) toolMeta`
- L43: `func toolKindFromName(name string, isMCP bool) string`
- L71: `func (c *SDKClient) makeHandler(sid string) func(sdk.SessionEvent)`
- L184: `func historyEvents(sid string, raw []sdk.SessionEvent) []Event`
- L214: `func normalizeUsage(d *sdk.AssistantUsageData) UsageData`
- L233: `func normalizeElicitFields(schema *sdk.ElicitationSchema) []ElicitField`
- L258: `func normalizeElicitField(name string, raw any) ElicitField`
- L278: `func elicitStr(m map[string]any, key string) string`
- L287: `func elicitStrSlice(v any) []string`
- L303: `func elicitDefault(v any) string`
- L324: `func sessionError(d *sdk.SessionErrorData) error`
- L336: `func planChangeText(op sdk.PlanChangedOperation) string`
- L351: `func compactionSummary(d *sdk.SessionCompactionCompleteData) string`
- L365: `func describePermission(req sdk.PermissionRequest) string`
- L380: `func permWriteFields(req sdk.PermissionRequest) (file, intention, diff string)`
- L390: `func summarizeArgs(args any) string`
- L412: `func stringField(m map[string]any, key string) (string, bool)`
- L425: `func toolResultText(d *sdk.ToolExecutionCompleteData) string`
- L442: `func oneLine(s string) string`
- L446: `func clip(s string, n int) string`
- L457: `func deref(p *int64) int64`
- L464: `func derefStr(p *string) string`
- L474: `func subagentSummary(durationMs, totalTokens *int64) string`
- L486: `func humanTokenCount(n int64) string`

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

### types.go (158 LOC)
- L22: `type Skill struct`
- L35: `type Instruction struct`
- L45: `type Agent struct`
- L73: `func DefaultChatAgent() Agent`
- L83: `type MCPServer struct`
- L98: `func (s Skill) Validate() error`
- L112: `func (i Instruction) Validate() error`
- L123: `func (a Agent) Validate() error`
- L143: `func (m MCPServer) Validate() error`
- L153: `func validateID(kind, id string) error`

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

## internal/pause

### pause.go (224 LOC)
- L24: `type Kind string`
- L35: `type Cap string`
- L44: `type Action string`
- L54: `type Resolution struct`
- L63: `type Pause struct`
- L80: `func (p *Pause) Can(c Cap) bool`
- L92: `func (p *Pause) Wait() Resolution { return <-p.ch }`
- L97: `type Ledger struct`
- L107: `func NewLedger() *Ledger`
- L114: `func (l *Ledger) Register(p Pause) *Pause`
- L133: `func (l *Ledger) Resolve(id string, res Resolution) bool`
- L142: `func (l *Ledger) CancelAll(payload string) int`
- L159: `func (l *Ledger) Sweep(now time.Time) []string`
- L176: `func (l *Ledger) Get(id string) (Pause, bool)`
- L188: `func (l *Ledger) Pending() []Pause`
- L203: `func (l *Ledger) deliver(id string, res Resolution) bool`
- L220: `func (p *Pause) snapshot() Pause`

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

### credits.go (356 LOC)
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
- L325: `type Leash struct`
- L334: `func (l Leash) Active() bool { return l.MaxCredits > 0 || l.MaxTurns > 0 }`
- L341: `func (l Leash) Breached(credits float64, turns int64) bool`
- L353: `func FormatUSD(v float64) string { return fmt.Sprintf("$%.4f", v) }`
- L356: `func FormatCredits(v float64) string { return fmt.Sprintf("%.2f cr", v) }`

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

### history.go (428 LOC)
- L24: `type SpendRecord struct`
- L71: `func (r SpendRecord) Credits() float64 { return r.USD / USDPerCredit }`
- L75: `func (r SpendRecord) EstimateCredits() float64 { return r.Credits() }`
- L79: `func (r SpendRecord) HasReported() bool { return HasReported(r.AIU) }`
- L84: `func (r SpendRecord) ActualCredits() float64 { return ActualCredits(r.EstimateCredits(), r.AIU) }`
- L87: `func (r SpendRecord) Day() string { return r.At.UTC().Format("2006-01-02") }`
- L108: `type SpendStore struct`
- L118: `func LoadSpendStore(dir string) (*SpendStore, error)`
- L128: `func stampSpend(r SpendRecord) SpendRecord`
- L136: `type DayTotal struct`
- L145: `func DailyTotals(records []SpendRecord) []DayTotal`
- L183: `func MonthToDate(records []SpendRecord, now time.Time) Cost`
- L197: `type share struct`
- L211: `func shareBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) []share`
- L242: `type ModelShare struct`
- L252: `func ModelShares(records []SpendRecord) []ModelShare`
- L264: `type AgentShare struct`
- L278: `func AgentShares(records []SpendRecord) []AgentShare`
- L290: `func rootTurns(records []SpendRecord) []SpendRecord`
- L302: `type WorkflowShare struct`
- L314: `func WorkflowShares(records []SpendRecord) []WorkflowShare`
- L326: `type SubagentShare struct`
- L340: `func SubagentShares(records []SpendRecord) []SubagentShare`
- L363: `type SessionShare struct`
- L376: `func SessionShares(records []SpendRecord) []SessionShare`
- L388: `func WriteCSV(w io.Writer, records []SpendRecord) error`
- L426: `func csvFloat(v float64) string`

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

### runs.go (366 LOC)
- L25: `type RunLane struct`
- L45: `type RunRecord struct`
- L60: `func (r RunRecord) Credits() float64`
- L72: `func (r RunRecord) Duration() time.Duration`
- L84: `func (r RunRecord) TotalPauses() int`
- L96: `func (r RunRecord) TotalPausedDuration() time.Duration`
- L121: `type RunStore struct`
- L129: `func LoadRunStore(dir string) (*RunStore, error)`
- L139: `func stampRun(r RunRecord) RunRecord`
- L153: `type RunAggregate struct`
- L176: `func (a RunAggregate) FailureRate() float64`
- L192: `func RunAggregates(records []RunRecord) []RunAggregate`
- L247: `type LaneShare struct`
- L265: `func LaneShares(records []RunRecord) []LaneShare`
- L320: `func WriteRunsCSV(w io.Writer, records []RunRecord) error`
- L350: `func csvTime(t time.Time) string`
- L361: `func laterRun(s2, f2, s1, f1 time.Time) bool`

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

### commands.go (436 LOC)
- L23: `func parseCommand(input string) (name, args string, ok bool)`
- L34: `func (s *Server) runCommand(input string) string`
- L66: `func (s *Server) systemNote(text string) string`
- L75: `func (s *Server) clearConversation()`
- L105: `func (s *Server) cmdClear() string`
- L123: `func (s *Server) setMode(target, args string) bool`
- L147: `func (s *Server) cmdPlan(args string) string`
- L157: `func (s *Server) cmdAuto(args string) string`
- L166: `func (s *Server) cmdAsk(args string) string`
- L176: `func (s *Server) cmdEffort(arg string) string`
- L198: `func (s *Server) setEffort(effort string)`
- L215: `func (s *Server) cmdModel(name string) string`
- L230: `func (s *Server) setModel(name string)`
- L249: `func (s *Server) cmdAgent(arg string) string`
- L318: `func SeamSpec(cs ctxforge.SessionSpec, defModel, defEffort string, lookupEnv func(string) string, workspace string, autoApprove bool) copilot.SessionSpec`
- L336: `func (s *Server) compiledSpec(agentID string) copilot.SessionSpec`
- L350: `func (s *Server) applyAgentSpec(c copilot.SessionSpec, agentID string, leash telemetry.Leash, leashLabel string) string`
- L371: `func (s *Server) cmdCost() string`
- L392: `func (s *Server) cmdAttach(path string) string`
- L406: `func (s *Server) cmdNav(slug string) string`
- L411: `func isNavSlug(slug string) bool`
- L422: `func commandHelp() string`

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

### demo.go (157 LOC)
- L17: `func streamDemoReply(m *copilot.MockClient, prompt string)`
- L148: `func tokenize(s string) []string`

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

### forms.go (231 LOC)
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
- L171: `func leashCreditsStr(v float64) string`
- L178: `func (s *Server) handleAgentNew(w http.ResponseWriter, r *http.Request)  { agentCRUD.New(s, w, r) }`
- L179: `func (s *Server) handleAgentEdit(w http.ResponseWriter, r *http.Request) { agentCRUD.Edit(s, w, r) }`
- L181: `func agentFromForm(r *http.Request, id string) ctxforge.Agent`
- L199: `func parseLeashFloat(s string) float64`
- L207: `func parseLeashInt(s string) int`
- L215: `func (s *Server) handleAgentCreate(w http.ResponseWriter, r *http.Request) { agentCRUD.Create(s, w, r) }`
- L216: `func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request) { agentCRUD.Update(s, w, r) }`
- L219: `func parseCSV(s string) []string`

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

### hub.go (340 LOC)
- L30: `type Hub struct`
- L74: `type Options struct`
- L96: `func New(opts Options) *Hub`
- L136: `func (h *Hub) newSession(id string) *Server`
- L173: `func (h *Hub) session(w http.ResponseWriter, r *http.Request) *Server`
- L202: `func (h *Hub) bind(copilotID string, s *Server)`
- L211: `func (h *Hub) route(copilotID string) *Server`
- L227: `func (h *Hub) pump()`
- L237: `func (h *Hub) Handler() http.Handler`
- L333: `func (s *Server) Handler() http.Handler { return s.hub.Handler() }`
- L336: `func newID() string`

### instructions_import.go (65 LOC)
- L31: `func importInstructionFiles(dir string) []ctxforge.Instruction`
- L51: `func (s *Server) handleInstructionImport(w http.ResponseWriter, r *http.Request)`

### markdown.go (418 LOC)
- L51: `type Block interface`
- L56: `type headingBlock struct`
- L63: `type codeBlock struct`
- L69: `type listBlock struct`
- L76: `type quoteBlock struct`
- L83: `type calloutBlock struct`
- L90: `type hrBlock struct{}`
- L94: `type paragraphBlock struct`
- L101: `func renderMarkdown(src string) string`
- L109: `func parseBlocks(src string) []Block`
- L113: `func parseLines(lines []string) []Block`
- L212: `func listItemRe(line string) *regexp.Regexp`
- L223: `func renderBlocks(blocks []Block) string`
- L229: `func renderBlocksTo(b *strings.Builder, blocks []Block)`
- L235: `func (h headingBlock) renderTo(b *strings.Builder)`
- L240: `func (c codeBlock) renderTo(b *strings.Builder)`
- L256: `func (l listBlock) renderTo(b *strings.Builder)`
- L268: `func (q quoteBlock) renderTo(b *strings.Builder)`
- L274: `func (c calloutBlock) renderTo(b *strings.Builder)`
- L291: `func (hrBlock) renderTo(b *strings.Builder)`
- L295: `func (p paragraphBlock) renderTo(b *strings.Builder)`
- L308: `func isBlockStart(line string) bool`
- L325: `func isHR(line string) bool`
- L345: `func inline(s string) string`
- L410: `func safeURL(url string) bool`

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

### pages.go (201 LOC)
- L53: `func clampWindow(raw string) int`
- L70: `func (s *Server) renderPage(slug, window string) string`
- L109: `func (s *Server) chatPartial() string`
- L146: `func (s *Server) agentLabel(id string) string`
- L158: `func (s *Server) workflowLabel(id string) string`
- L169: `func (s *Server) handleRunsExport(w http.ResponseWriter, r *http.Request)`
- L182: `func addData(kind, noun string) map[string]any { return map[string]any{"Kind": kind, "Noun": noun} }`
- L184: `func def(s, fallback string) string`
- L192: `func truncate(s string, n int) string`

### palette.go (33 LOC)
- L15: `func commandPalette() string`

### pause.go (247 LOC)
- L21: `type escalateReq struct`
- L37: `func (s *Server) escalate(req escalateReq) string`
- L86: `func (s *Server) parkLane(session, message, pauseID string)`
- L103: `func (s *Server) closeLanePause(session string) { s.closeLanePauseLane(s.laneBySession(session)) }`
- L113: `func (s *Server) closeLanePauseLane(l *lane)`
- L128: `func (s *Server) resumeLane(session string, res pause.Resolution)`
- L144: `func (s *Server) laneBySession(session string) *lane`
- L160: `func (s *Server) pauseFrags() []fragment`
- L177: `func (s *Server) handlePause(w http.ResponseWriter, r *http.Request)`
- L197: `func escalateResult(res pause.Resolution) string`
- L216: `func cancelNote(res pause.Resolution) string`
- L228: `func renderPauses(pending []pause.Pause) string`
- L240: `func renderPauseForm(p pause.Pause) string`

### render.go (627 LOC)
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
- L337: `func subagentGlyph(st convo.SubagentStatus) string`
- L356: `func subagentHeader(entries []convo.SubagentView) string`
- L383: `func renderSubagents(entries []convo.SubagentView) string`
- L418: `func renderStatus(text string, active bool, startMs int64) string`
- L426: `func renderCtx(cur, limit int64, compacting bool) string`
- L461: `func renderStatline(s *Server) string`
- L522: `func statlineForecast(s *Server, now time.Time) (show bool, short string, warn bool, title string)`
- L536: `func humanTokens(n int64) string`
- L552: `func renderCostFooter(credits float64, budget telemetry.Budget) string`
- L572: `func renderActualCostFooter(a telemetry.ActualSpend, budget telemetry.Budget) string`
- L604: `func renderBudgetForm(projected, capCredits float64) string`
- L615: `func renderLeashForm(label, reason string) string`
- L621: `func clampLines(s string, n int) string`

### runs.go (213 LOC)
- L28: `func (s *Server) runsPartial(window int) string`
- L62: `func windowRuns(records []telemetry.RunRecord, window int) []telemetry.RunRecord`
- L91: `func (s *Server) runSummaryRow(a telemetry.RunAggregate) map[string]any`
- L110: `func (s *Server) laneShareRow(l telemetry.LaneShare) map[string]any`
- L132: `func (s *Server) runRow(r telemetry.RunRecord, window int) map[string]any`
- L165: `func pausedFor(ms int64) string`
- L180: `func humanDuration(d time.Duration) string`
- L208: `func runOutcomeGlyph(outcome string) (glyph, state string)`

### server.go (996 LOC)
- L28: `type Server struct`
- L126: `func (s *Server) subscribe() chan fragment`
- L135: `func (s *Server) unsubscribe(ch chan fragment)`
- L146: `func (s *Server) broadcast(frags []fragment)`
- L165: `func (s *Server) broadcastSendFailure(err error)`
- L170: `type indexData struct`
- L185: `type navItem struct`
- L193: `type navGroup struct`
- L203: `func navGroups() []navGroup`
- L221: `func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request)`
- L244: `func (s *Server) ensureSession(ctx context.Context) (string, error)`
- L263: `func (s *Server) handleSend(w http.ResponseWriter, r *http.Request)`
- L377: `type budgetGate struct`
- L393: `func (s *Server) pendingLeashGate(prompt string, attachments []string) *budgetGate`
- L401: `func (s *Server) leashGate(prompt string, attachments []string, agentID string, leash telemetry.Leash) *budgetGate`
- L414: `func leashReason(leash telemetry.Leash, credits float64, turns int64) string`
- L427: `func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request)`
- L495: `func (s *Server) dispatch(ctx context.Context, sessionID, prompt string, attachments []string) error`
- L514: `func (s *Server) sendFailedOOB(err error) string`
- L526: `func queuedStatus(n int) string`
- L533: `func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request)`
- L554: `func (s *Server) handlePerm(w http.ResponseWriter, r *http.Request)`
- L584: `func (s *Server) dropLanePerm(id string) string`
- L601: `func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request)`
- L621: `func (s *Server) handlePlanReview(w http.ResponseWriter, r *http.Request)`
- L654: `func (s *Server) handleElicit(w http.ResponseWriter, r *http.Request)`
- L692: `func elicitContent(fields []copilot.ElicitField, form url.Values) map[string]any`
- L722: `func (s *Server) handlePage(w http.ResponseWriter, r *http.Request)`
- L735: `func (s *Server) handleSkillToggle(w http.ResponseWriter, r *http.Request)`
- L741: `func (s *Server) handleSkillDelete(w http.ResponseWriter, r *http.Request)`
- L745: `func (s *Server) handleInstructionToggle(w http.ResponseWriter, r *http.Request)`
- L751: `func (s *Server) handleInstructionDelete(w http.ResponseWriter, r *http.Request)`
- L755: `func (s *Server) handleAgentSelect(w http.ResponseWriter, r *http.Request)`
- L787: `func (s *Server) handleModelSelect(w http.ResponseWriter, r *http.Request)`
- L794: `func (s *Server) handleEffortSelect(w http.ResponseWriter, r *http.Request)`
- L803: `func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request)`
- L826: `func (s *Server) oobTimeline() string`
- L831: `func oobStatus(text string, active bool, startMs int64) string`
- L837: `func (s *Server) oobBudget() string`
- L843: `func (s *Server) budgetFrag() fragment`
- L849: `func (s *Server) renderGate() string`
- L861: `func (s *Server) statusFrag(text string, active bool) fragment`
- L871: `func (s *Server) ctxFrag() fragment`
- L877: `func (s *Server) statFrag() fragment`
- L884: `func (s *Server) oobStat() string`
- L890: `func (s *Server) subagentsFrag() fragment`
- L901: `func (s *Server) attentionFrag() fragment`
- L912: `func attentionMarker(n int) string`
- L918: `func nowMs() int64 { return time.Now().UnixMilli() }`
- L923: `func dropByID[T any](sl []T, id string, key func(T) string) []T`
- L934: `func findByID[T any](sl []T, id string, key func(T) string) (T, bool)`
- L944: `func permID(p copilot.PermissionRequest) string { return p.ID }`
- L945: `func inputID(p copilot.InputRequest) string     { return p.ID }`
- L946: `func planID(p copilot.PlanRequest) string       { return p.ID }`
- L947: `func elicitID(e copilot.ElicitRequest) string   { return e.ID }`
- L952: `func (s *Server) dropPerm(id string)  { s.perms = dropByID(s.perms, id, permID) }`
- L953: `func (s *Server) dropInput(id string) { s.inputs = dropByID(s.inputs, id, inputID) }`
- L954: `func (s *Server) dropPlan(id string)  { s.plans = dropByID(s.plans, id, planID) }`
- L955: `func (s *Server) dropElicit(id string)`
- L961: `func (s *Server) findElicit(id string) (copilot.ElicitRequest, bool)`
- L966: `func firstNonEmpty(vals []string) string`
- L979: `func (s *Server) editForge(fn func() error) error`
- L993: `func (s *Server) writePartial(w http.ResponseWriter, html string)`

### session.go (503 LOC)
- L15: `type liveKind int`
- L28: `func (s *Server) handleEvent(e copilot.Event) []fragment`
- L305: `type spendTag struct`
- L328: `func (s *Server) recordUsage(u copilot.UsageData, tag spendTag) telemetry.Cost`
- L394: `func (s *Server) leashFor(agentID string) (telemetry.Leash, bool)`
- L410: `func (s *Server) handleSubagentStream(e copilot.Event) []fragment`
- L469: `func (s *Server) recordSubagentUsage(e copilot.Event) []fragment`
- L485: `func (s *Server) timelineFragments() []fragment`
- L498: `func toolID(e copilot.Event) string`

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

### subagent_overlay.go (176 LOC)
- L29: `func subagentEvent(spawnID string) string { return "subagent-" + spawnID }`
- L34: `func (s *Server) subagentOverlayFrag(instanceID string) (fragment, bool)`
- L47: `func (s *Server) handleSubagentOverlay(w http.ResponseWriter, r *http.Request)`
- L66: `func (s *Server) handleSubagentSteer(w http.ResponseWriter, r *http.Request)`
- L108: `func (s *Server) subagentTranscriptFrag(v convo.SubagentView) fragment`
- L119: `func renderSubagentOverlay(v convo.SubagentView, pauses []pause.Pause) string`
- L150: `func renderSubagentTranscript(v convo.SubagentView) string`
- L168: `func (s *Server) pausesFor(v convo.SubagentView) []pause.Pause`

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

### telemetry_render.go (431 LOC)
- L12: `func (s *Server) telemetryPartial(window int) string`
- L83: `func (s *Server) spendTrend(window int) (days, shares []map[string]any, hasHistory bool)`
- L140: `func (s *Server) spendShares(now time.Time) (agents, workflows, subagents []map[string]any)`
- L175: `func agentKey(r telemetry.SpendRecord) string { return r.AgentID }`
- L177: `func workflowKey(r telemetry.SpendRecord) string { return r.WorkflowID }`
- L195: `func (s *Server) workflowReconcile() []map[string]any`
- L216: `func (s *Server) reconcileRow(r telemetry.WorkflowRecon) map[string]any`
- L233: `func (s *Server) laneReconcile() []map[string]any`
- L255: `func (s *Server) laneReconcileRow(r telemetry.LaneRecon) map[string]any`
- L272: `func (s *Server) estimateDrift() []map[string]any`
- L298: `func bucketTrajectories(bs []telemetry.BucketProjection, now time.Time) map[string]string`
- L314: `func bucketTrajectoryText(p telemetry.Projection, now time.Time) string`
- L333: `func daysLeftInMonth(now time.Time) int`
- L344: `func shareRow(label string, credits, fraction float64) map[string]any`
- L356: `func forecastView(fc telemetry.Projection, allowance float64, now time.Time) map[string]any`
- L380: `func forecastSoon(exhaust, now time.Time) bool`
- L390: `func plural(n int, one, many string) string`
- L400: `func (s *Server) handleSpendExport(w http.ResponseWriter, r *http.Request)`
- L417: `func (s *Server) handleReconcileExport(w http.ResponseWriter, r *http.Request)`

### tmpl.go (55 LOC)
- L44: `func trusted(s string) template.HTML { return template.HTML(s) } //nolint:gosec // composed from escaped fragments`
- L49: `func frag(name string, data any) string`

### workflow.go (1196 LOC)
- L30: `type laneStatus int`
- L46: `func settled(st laneStatus) bool`
- L52: `type lane struct`
- L86: `func (l *lane) toolStart(id, name, args string)`
- L100: `func (l *lane) toolProgress(id, msg string)`
- L107: `func (l *lane) toolEnd(id, result string, success bool)`
- L118: `func (l *lane) toolByID(id string) *convo.ToolView`
- L130: `func (l *lane) dropPerm(id string) bool`
- L144: `type workflowRun struct`
- L163: `func newWorkflowRun(wf ctxforge.Workflow, steps []ctxforge.CompiledStep, specs []copilot.SessionSpec) *workflowRun`
- L178: `func (r *workflowRun) start() []int`
- L197: `func (r *workflowRun) evalWhen(l *lane) (satisfied, ready bool)`
- L218: `func skipDetail(w *ctxforge.StepCondition) string`
- L237: `func (r *workflowRun) evalPending() []int`
- L268: `func (r *workflowRun) handoffPrompt(idx int) string`
- L288: `func (r *workflowRun) laneFor(sessionID string) *lane`
- L309: `func (l *lane) appendText(s string) { l.text += s }`
- L313: `func (r *workflowRun) finishLane(l *lane, detail string) []int`
- L325: `func (r *workflowRun) failLane(l *lane, msg string) []int`
- L341: `func (r *workflowRun) abort() []string`
- L365: `func (r *workflowRun) advance(l *lane) []int`
- L390: `func (r *workflowRun) allSettled() bool`
- L405: `func (s *Server) handleWorkflowRun(w http.ResponseWriter, r *http.Request)`
- L425: `func (s *Server) handleRunRerun(w http.ResponseWriter, r *http.Request)`
- L440: `func (s *Server) handleRunAbort(w http.ResponseWriter, r *http.Request)`
- L453: `func (s *Server) abortRun(ctx context.Context)`
- L493: `func (s *Server) launchWorkflow(id string) bool`
- L532: `func (s *Server) workflowLaneSpec(cs ctxforge.SessionSpec) copilot.SessionSpec`
- L543: `func (s *Server) launchLanes(run *workflowRun, idxs []int)`
- L551: `func (s *Server) startLane(run *workflowRun, idx int)`
- L581: `func (s *Server) laneError(run *workflowRun, l *lane, err error)`
- L595: `func (s *Server) handleRunEvent(run *workflowRun, e copilot.Event) []fragment`
- L702: `func (s *Server) runFrags(run *workflowRun, done bool) []fragment`
- L735: `func (s *Server) recordRun(run *workflowRun)`
- L748: `func runRecord(run *workflowRun) telemetry.RunRecord`
- L769: `func laneStatusName(st laneStatus) string`
- L787: `func (l *lane) costDetail() string`
- L795: `func (s *Server) lanesFrag() fragment`
- L802: `func renderLanes(run *workflowRun) string`
- L826: `func laneToolsHTML(tools []*convo.ToolView) template.HTML`
- L837: `func lanePermsHTML(perms []copilot.PermissionRequest) template.HTML`
- L848: `func laneGlyph(st laneStatus) (glyph, state string)`
- L855: `func glyphFor(status string) (glyph, state string)`
- L886: `func streamDemoLane(m *copilot.MockClient, sid, prompt string, escalate func(escalateReq) string)`
- L949: `func firstLine(s string) string`
- L960: `func (s *Server) workflowsPartial() string`
- L1016: `func renderWorkflowForm(w ctxforge.Workflow, isNew bool, errMsg string) string`
- L1037: `func stepsToText(steps []ctxforge.WorkflowStep) string`
- L1057: `func stepsFromText(raw string) []ctxforge.WorkflowStep`
- L1086: `func splitStepLine(line string) (head, prompt string)`
- L1105: `func formatStepCondition(c *ctxforge.StepCondition) string`
- L1123: `func parseStepCondition(spec string) *ctxforge.StepCondition`
- L1143: `func workflowFromForm(r *http.Request, id string) ctxforge.Workflow`
- L1153: `func (s *Server) handleWorkflowNew(w http.ResponseWriter, r *http.Request)`
- L1157: `func (s *Server) handleWorkflowEdit(w http.ResponseWriter, r *http.Request)`
- L1172: `func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request)`
- L1181: `func (s *Server) handleWorkflowUpdate(w http.ResponseWriter, r *http.Request)`
- L1191: `func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request)`

