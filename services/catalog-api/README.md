# catalog-api

Serviço HTTP responsável pelos produtos. Os produtos são carregados em memória na
inicialização a partir de [`seed/products.json`](seed/products.json). O arquivo é embutido no
binário (`go:embed`), então não precisa ser copiado para a imagem.

## Variáveis de ambiente

| Variável       | Obrigatória | Default       | Descrição                                          |
|----------------|-------------|---------------|----------------------------------------------------|
| `PORT`         | não         | `8080`        | Porta HTTP (escuta em `0.0.0.0`).                  |
| `SERVICE_NAME` | não         | `catalog-api` | Campo `service` dos logs e da rota `/version`.     |
| `LOG_LEVEL`    | não         | `info`        | `debug`, `info`, `warn` ou `error`.                |
| `APP_VERSION`  | não         | `dev`         | Versão exposta em `/catalog/version` e nos logs.   |

## Rotas

| Método | Rota                     | Resposta                                          |
|--------|--------------------------|---------------------------------------------------|
| GET    | `/catalog/health`        | `200 {"status":"ok"}`                             |
| GET    | `/catalog/version`       | `200 {"service":"catalog-api","version":"..."}`   |
| GET    | `/catalog/products`      | `200` lista de produtos                           |
| GET    | `/catalog/products/{id}` | `200` produto, ou `404 {"error":"product not found"}` |

Modelo do produto: `{"id": "p-001", "name": "...", "price": 349.9, "active": true}`.
O seed tem 10 produtos. `p-009` e `p-010` estão com `active=false`, para testar o 422 no orders-api.

## Comportamento

- **Logs:** JSON no stdout, uma linha por evento, com `timestamp`, `level`, `service`, `version`,
  `message` e `requestId` (quando houver). Cada requisição gera uma linha `request completed`;
  o health check só é logado com `LOG_LEVEL=debug`.
- **Correlation ID:** o `X-Request-Id` de entrada é reutilizado; sem ele, é gerado um UUID.
  O ID volta no header `X-Request-Id` da resposta.
- **Graceful shutdown:** no SIGTERM (ou SIGINT), o serviço para de aceitar conexões e espera
  até 18s pelas requisições em andamento.

## Rodando localmente

```sh
go run .
curl -s localhost:8080/catalog/products/p-001
```
