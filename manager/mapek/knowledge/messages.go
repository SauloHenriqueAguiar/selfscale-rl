package knowledge

// Mensagens trocadas entre as etapas (Sintoma, Solicitação de Mudança, Plano
// de Adaptação, Resultado da Execução) e o catálogo de sintomas.

import "time"

// Delta é a variação de réplicas decidida pelo Analisador.
type Delta int

// Actions devolve o conjunto fixo de ações do agente, na ordem das colunas da Q-table.
// É uma função (e não uma variável) para não expor estado global mutável.
func Actions() []Delta { return []Delta{-1, 0, +1, +2} }

// Traffic é a leitura bruta do tráfego, nas unidades da fonte (Prometheus).
type Traffic struct {
	P95Seconds float64 // NaN quando não houve requisições na janela
	RPS        float64
	ErrorRatio float64 // fração de 0 a 1
}

// SymptomKind é a categoria do sintoma (catálogo de sintomas da Base de Conhecimento).
type SymptomKind string

// Catálogo de sintomas.
const (
	KindViolation SymptomKind = "violation"
	KindRisk      SymptomKind = "risk"
	KindIdle      SymptomKind = "idle"
	KindOK        SymptomKind = "ok"
	KindInvalid   SymptomKind = "invalid"
)

// Symptom é a mensagem Monitor → Analisador (canal chMA).
type Symptom struct {
	T             time.Time
	Cycle         int
	P95Ms         float64
	ErrorPct      float64
	RPS           float64
	CPUPct        float64 // média do frontend, em % do requests.cpu
	Replicas      int     // spec.replicas
	ReadyReplicas int     // status.readyReplicas
	Valid         bool
	Kind          SymptomKind // preenchido pelo Analisador
}

// Motivos de uma ChangeRequest.
const (
	ReasonInvalidData = "invalid-data"
	ReasonObserve     = "observe"
	ReasonPolicy      = "policy"
)

// ChangeRequest é a mensagem Analisador → Planejador (canal chAP).
// Carrega o sintoma e os dados da análise para o registro do ciclo.
type ChangeRequest struct {
	T       time.Time
	Delta   Delta
	State   State
	Reason  string
	Symptom Symptom
	Reward  float64 // recompensa da ação executada no ciclo anterior (NaN se não houve)
	Epsilon float64
}

// StepKind identifica um passo do plano; o Executor o traduz em um comando.
type StepKind string

// Passos possíveis de um plano.
const (
	StepScale      StepKind = "scale"
	StepAwaitReady StepKind = "await-ready"
	StepConfirm    StepKind = "confirm"
)

// Step é um passo do plano de adaptação.
type Step struct {
	Kind     StepKind
	Replicas int
}

// Motivos de bloqueio de um plano.
const (
	BlockedCooldown    = "cooldown"
	BlockedPending     = "pending"
	BlockedBounds      = "bounds"
	BlockedInvalidData = "invalid-data"
)

// AdaptationPlan é a mensagem Planejador → Executor (canal chPE).
type AdaptationPlan struct {
	T         time.Time
	Target    int
	Steps     []Step
	Timeout   time.Duration
	Fallback  int
	Empty     bool
	BlockedBy string
	Request   ChangeRequest
}

// ExecutionResult é o resultado do Executor, gravado na Base de Conhecimento.
type ExecutionResult struct {
	T             time.Time
	ExecutedDelta Delta
	Success       bool
	TimeToReady   time.Duration
	RolledBack    bool
}

// CycleRecord é uma linha do histórico (um ciclo MAPE-K). O plano carrega a
// solicitação, que carrega o sintoma: o registro tem a cadeia completa do ciclo.
type CycleRecord struct {
	Plan     AdaptationPlan
	Executed Delta
	// Finished é a execução que terminou durante este ciclo (nil se nenhuma);
	// a prontidão é acompanhada em segundo plano e pode levar mais de um ciclo.
	Finished *ExecutionResult
}
