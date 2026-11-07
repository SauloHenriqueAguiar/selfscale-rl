# Sistema gerenciado — Online Boutique

Tudo o que é **adaptado** pelo SelfScale-RL fica aqui. O código da loja não é
alterado: só manifestos Kubernetes, aplicados pelo Terraform
(`infra/terraform/platform/modules/boutique` e `hpa_baseline`).

| Pasta | Conteúdo |
|---|---|
| `boutique/base/` | manifesto oficial `microservices-demo` **v0.10.2**, sem modificações |
| `boutique/overlay/` | ajustes via kustomize: namespace `boutique`, remove o `loadgenerator` e o `frontend-external`, 1 réplica em tudo |
| `hpa/` | HPA do `frontend` (CPU 70%, 1–5 réplicas): baseline E3, nunca ligado junto com o SelfScale-RL |

Os recursos de CPU (frontend apertado, dependências folgadas) vêm das variáveis
do Terraform (`frontend_cpu_*`, `deps_cpu_*`), para serem calibrados no piloto.

Elemento adaptado: **`Deployment/frontend`** (réplicas 1–5). Os demais serviços
ficam com 1 réplica fixa e CPU de sobra.

Os **sensores** que observam a loja (proxy de medição `manager/sensors/probe`,
metrics-server e Prometheus) pertencem ao sistema gerenciador e são implantados
pelos módulos `probe` e `monitoring`.
