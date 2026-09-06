# Kindness Chain

One sentence from you. One dime from me. Verified on-chain.

Anyone can add one sentence about a kind act they did or will do today. No signup, no wallet. Each link is written to Solana devnet as an SPL Memo transaction that references the previous link's signature, so the whole chain can be checked by anyone with a block explorer. For every confirmed link I donate $0.10 to the International Institute of Minnesota, up to $50. The chain is the receipt.

## Run locally

You need Go 1.26 and Node 24.

```
cp .env.example .env    # then paste your devnet keypair into SOLANA_KEYPAIR
make build-web          # builds the frontend into web/dist
make run                # serves everything on http://localhost:8080
```

For frontend work, run `make dev` in a second terminal. Vite serves the app on http://localhost:5173 and proxies the API to the Go server.

`make test` runs vet and the tests.

## License

MIT
