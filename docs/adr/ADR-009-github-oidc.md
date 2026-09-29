# ADR-009 GitHub OIDC authentication

Status: accepted

`deploy-azure.yml` and `deploy-aks.yml` request `id-token: write` and use `azure/login` with federated credentials. They do not store a client secret. Production deploys use the `production` GitHub environment so approval rules can be attached there. See [../deployment/github-oidc.md](../deployment/github-oidc.md).
