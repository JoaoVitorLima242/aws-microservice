# notifier-worker

Worker sem HTTP. Consome os eventos `OrderCreated` do SQS e "envia" a notificação ao cliente.
Por enquanto, o envio é só um log `notification sent`.

## Variáveis de ambiente

| Variável            | Obrigatória | Default           | Descrição |
|---------------------|-------------|-------------------|-----------|
| `QUEUE_URL`         | **sim**     | —                 | URL da fila SQS consumida. |
| `AWS_REGION`        | **sim**     | —                 | Região do cliente SQS. |
| `AWS_ENDPOINT_URL`  | não         | —                 | Endpoint customizado (ex: LocalStack `http://localstack:4566`). |
| `MAX_MESSAGES`      | não         | `10`              | Mensagens por ReceiveMessage (1 a 10). |
| `WAIT_TIME_SECONDS` | não         | `20`              | Tempo de long polling (0 a 20). |
| `FAIL_RATE`         | não         | `0`               | Fração de mensagens que falham de propósito (0 a 1). |
| `SERVICE_NAME`      | não         | `notifier-worker` | Campo `service` dos logs. |
| `LOG_LEVEL`         | não         | `info`            | `debug`, `info`, `warn` ou `error`. |
| `APP_VERSION`       | não         | `dev`             | Versão incluída nos logs. |

**Credenciais AWS:** vêm da cadeia padrão do SDK (env, `~/.aws`, task role do ECS etc.), nunca do código.

A configuração é validada na inicialização. Se algo estiver inválido, o processo loga ERROR e sai com código 1.

### Permissões IAM necessárias

Na fila `QUEUE_URL`: `sqs:ReceiveMessage`, `sqs:DeleteMessage`.

## Processamento de cada mensagem

| Situação                                    | Log                                                  | Apaga a mensagem? |
|---------------------------------------------|------------------------------------------------------|-------------------|
| Sucesso                                     | INFO `notification sent` (orderId, customerEmail, requestId) | sim |
| `eventId` já processado (duplicada)         | INFO `duplicate event ignored`                       | sim |
| JSON inválido                               | ERROR `invalid message: malformed JSON...`           | **não** |
| JSON válido com formato inesperado¹         | ERROR `invalid message: unexpected format...`        | **não** |
| Falha simulada (`FAIL_RATE`) ou outro erro  | ERROR `failed to process message, it will be retried` | **não** |

¹ `eventType` diferente de `OrderCreated`, ou `eventId`, `data.orderId` ou `data.customerEmail` vazios.

Quando a mensagem não é apagada, ela volta para a fila depois do *visibility timeout*. Depois de
`maxReceiveCount` recebimentos, vai para a DLQ (configurada na redrive policy da fila). Todos os
logs de mensagem trazem `messageId` e `receiveCount` (`ApproximateReceiveCount`), para você ver
o retry acontecendo. O `requestId` do evento entra nos logs, então dá para seguir um pedido desde
o `POST /orders`.

**Idempotência:** os `eventId`s processados ficam em um set em memória. O ID é marcado antes do
delete: se o delete falhar, a cópia reentregue é reconhecida como duplicada e só é apagada.
O set é perdido no restart e não é compartilhado entre réplicas, ou seja, a deduplicação é
"best effort", como pedido. Ele também cresce sem limite enquanto o processo vive.

## Shutdown

No SIGTERM (ou SIGINT), o worker para de buscar mensagens novas. Um long poll em andamento é
cancelado na hora. O lote já recebido é processado e apagado até o fim, e o processo sai.
Se o lote não terminar em 18s, o processo sai com código 1.

## Rodando localmente

```sh
AWS_REGION=us-east-1 AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test \
AWS_ENDPOINT_URL=http://localhost:4566 \
QUEUE_URL=http://localhost:4566/000000000000/orders \
FAIL_RATE=0.3 go run .
```
