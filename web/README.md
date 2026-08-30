# Navego Web

Dashboard Next.js do Navego. Autenticação e operações administrativas passam
por Route Handlers BFF; o token do PocketBase permanece em cookie HttpOnly.

## Desenvolvimento

```sh
npm run dev
```

O control plane deve estar em `http://127.0.0.1:8090` ou na URL configurada em
`NAVEGO_CONTROL_URL`.

## Validação

```sh
npx tsc --noEmit
npm run lint
npm run build
```
