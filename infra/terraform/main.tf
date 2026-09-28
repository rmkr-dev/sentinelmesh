module "resource_group" {
  source   = "./modules/resource-group"
  name     = "${var.prefix}-rg"
  location = var.location
  tags     = var.tags
}

module "network" {
  count               = local.enable_net ? 1 : 0
  source              = "./modules/network"
  name                = "${var.prefix}-vnet"
  location            = var.location
  resource_group_name = module.resource_group.name
  tags                = var.tags
}

module "identity" {
  source              = "./modules/identity"
  name                = "${var.prefix}-collector"
  location            = var.location
  resource_group_name = module.resource_group.name
  tags                = var.tags
}

module "monitoring" {
  source              = "./modules/monitoring"
  name                = var.prefix
  location            = var.location
  resource_group_name = module.resource_group.name
  alert_email         = var.alert_email
  tags                = var.tags
}

module "acr" {
  source              = "./modules/acr"
  name                = replace("${var.prefix}acr", "-", "")
  location            = var.location
  resource_group_name = module.resource_group.name
  tags                = var.tags
}

module "keyvault" {
  source              = "./modules/keyvault"
  name                = substr(replace("${var.prefix}kv", "-", ""), 0, 24)
  location            = var.location
  resource_group_name = module.resource_group.name
  tenant_id           = module.identity.tenant_id
  tags                = var.tags
}

module "aks" {
  count               = local.enable_aks ? 1 : 0
  source              = "./modules/aks"
  name                = "${var.prefix}-aks"
  location            = var.location
  resource_group_name = module.resource_group.name
  dns_prefix          = "${var.prefix}-aks"
  subnet_id           = local.enable_net ? module.network[0].aks_subnet_id : null
  tags                = var.tags
}

module "grafana" {
  count               = local.enable_graf ? 1 : 0
  source              = "./modules/grafana"
  name                = "${var.prefix}-grafana"
  location            = var.location
  resource_group_name = module.resource_group.name
  tags                = var.tags
}

resource "azurerm_role_assignment" "collector_metrics" {
  scope                = module.monitoring.workspace_id
  role_definition_name = "Monitoring Metrics Publisher"
  principal_id         = module.identity.principal_id
}

resource "azurerm_role_assignment" "acr_pull" {
  count                = local.enable_aks ? 1 : 0
  scope                = module.acr.id
  role_definition_name = "AcrPull"
  principal_id         = module.aks[0].kubelet_identity_object_id
}
