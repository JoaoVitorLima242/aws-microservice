# orders-api

Serviço HTTP responsável pelos pedidos. Consulta o catalog-api para obter o preço, salva o
pedido (em memória ou no DynamoDB) e publica o evento `OrderCreated` no SQS.

## Variáveis de ambiente

| Variável           | Obrigatória                    | Default      | Descrição |
|--------------------|--------------------------------|--------------|-----------|
| `PORT`             | não                            | `8080`       | Porta HTTP (escuta em `0.0.0.0`). |
| `CATALOG_URL`      | **sim**                        | —            | URL base do catalog-api, ex: `http://catalog-api:8080`. |
| `STORAGE`          | não                            | `memory`     | `memory` ou `dynamodb`. |
| `ORDERS_TABLE`     | se `STORAGE=dynamodb`          | —            | Nome da tabela DynamoDB (partition key `id`, tipo String). |
| `QUEUE_URL`        | não (veja abaixo)              | —            | URL da fila SQS onde o `OrderCreated` é publicado. |
| `AWS_REGION`       | se usar DynamoDB ou SQS        | —            | Região dos clientes AWS. |
| `AWS_ENDPOINT_URL` | não                            | —            | Endpoint customizado para DynamoDB e SQS (ex: LocalStack `http://localstack:4566`). |
| `SERVICE_NAME`     | não                            | `orders-api` | Campo `service` dos logs e da rota `/version`. |
| `LOG_LEVEL`        | não                            | `info`       | `debug`, `info`, `warn` ou `error`. |
| `APP_VERSION`      | não                            | `dev`        | Versão exposta em `/orders/version` e nos logs. |

**Credenciais AWS:** nunca vêm do código. O SDK usa a cadeia padrão de credenciais: variáveis
`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, `~/.aws`, task role do ECS, instance profile etc.
Para o LocalStack, basta `AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test`.
O SDK também entende as variáveis por serviço, como `AWS_ENDPOINT_URL_SQS` e
`AWS_ENDPOINT_URL_DYNAMODB`, que valem quando `AWS_ENDPOINT_URL` não está definida.

**Sem `QUEUE_URL`:** o serviço sobe com um aviso e cada publicação falha, gerando log ERROR.
Os pedidos ficam com `eventPublished=false`. Isso é útil para desenvolver sem fila.

A configuração é validada na inicialização. Se algo estiver inválido, o processo loga ERROR e sai com código 1.

### Permissões IAM necessárias

- DynamoDB (na tabela `ORDERS_TABLE`): `dynamodb:PutItem`, `dynamodb:GetItem`,
  `dynamodb:UpdateItem`, `dynamodb:Scan`
- SQS (na fila `QUEUE_URL`): `sqs:SendMessage`

## Rotas

| Método | Rota              | Resposta |
|--------|-------------------|----------|
| GET    | `/orders/health`  | `200 {"status":"ok"}` |
| GET    | `/orders/version` | `200 {"service":"orders-api","version":"..."}` |
| POST   | `/orders`         | `201` pedido criado (veja abaixo) |
| GET    | `/orders`         | `200` lista de pedidos, do mais recente para o mais antigo |
| GET    | `/orders/{id}`    | `200` pedido, ou `404` |

### POST /orders

```json
{"productId": "p-001", "quantity": 2, "customerEmail": "ana@example.com"}
```

| Situação                                              | Status |
|-------------------------------------------------------|--------|
| Body inválido, `quantity < 1` ou e-mail inválido      | `400`  |
| Produto não existe ou está `active=false`             | `422`  |
| Catalog fora do ar, timeout ou resposta inesperada    | `503`  |
| Pedido criado (mesmo que a publicação na fila falhe)  | `201`  |

O catalog é chamado com timeout de 2s por tentativa e 1 retry (2 tentativas no total, com
100ms entre elas). O 404 do catalog não é repetido. O `X-Request-Id` é propagado para o catalog.

Resposta 201:

```json
{
  "id": "07ea3ae8-209f-4812-b83b-d3934ec815c2",
  "productId": "p-001",
  "quantity": 2,
  "unitPrice": 349.9,
  "total": 699.8,
  "customerEmail": "ana@example.com",
  "status": "CREATED",
  "createdAt": "2026-10-02T18:30:34.646817Z",
  "eventPublished": true
}
```

Os valores monetários usam decimal (sem erro de ponto flutuante). No DynamoDB eles ficam como Number.

O pedido é salvo e o evento é publicado com um contexto separado do request, com 5s de timeout.
Se o cliente desconectar no meio, o pedido não fica pela metade.

### Mensagem publicada no SQS

```json
{
  "eventType": "OrderCreated",
  "eventId": "<uuid>",
  "occurredAt": "2026-10-02T18:30:34.83Z",
  "requestId": "<correlation id>",
  "data": {"orderId": "...", "productId": "p-001", "quantity": 2, "total": 699.8, "customerEmail": "ana@example.com"}
}
```

A mensagem também leva os message attributes `eventType` e `requestId`.

## Comportamento

- **Logs:** JSON no stdout, uma linha por evento, com `timestamp`, `level`, `service`, `version`,
  `message` e `requestId`. Respostas 5xx geram o `request completed` em nível ERROR.
- **Graceful shutdown:** no SIGTERM, o serviço para de aceitar conexões e espera até 18s pelas
  requisições em andamento.
- **`GET /orders` com DynamoDB:** faz um Scan da tabela inteira. Serve para estudo; em produção
  seria preciso paginar.

## Rodando localmente

```sh
# só memória, sem fila
CATALOG_URL=http://localhost:8081 PORT=8082 go run .

# DynamoDB + SQS no LocalStack
AWS_REGION=us-east-1 AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
AWS_ENDPOINT_URL=http://localhost:4566 \
STORAGE=dynamodb ORDERS_TABLE=orders \
QUEUE_URL=http://localhost:4566/000000000000/orders \
CATALOG_URL=http://localhost:8081 PORT=8082 go run .
```
