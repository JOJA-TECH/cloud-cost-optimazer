resource "random_uuid" "experiment" {}

# ---------------------------------------------------------------------------
# Resource group for the collector VM (vm_subscription_id, azurerm.vm).
# Created and destroyed as one unit with the VM.
# ---------------------------------------------------------------------------
resource "azurerm_resource_group" "vm" {
  provider = azurerm.vm
  name     = local.vm.resource_group
  location = local.vm.location
  tags     = local.tags
}

# ---------------------------------------------------------------------------
# Virtual network and application subnet (vm subscription).
# ---------------------------------------------------------------------------
resource "azurerm_virtual_network" "vm" {
  provider            = azurerm.vm
  name                = local.vm.vnet
  location            = azurerm_resource_group.vm.location
  resource_group_name = azurerm_resource_group.vm.name
  address_space       = ["10.42.0.0/16"]
  tags                = local.tags
}

resource "azurerm_subnet" "app" {
  provider             = azurerm.vm
  name                 = local.vm.subnet
  resource_group_name  = azurerm_resource_group.vm.name
  virtual_network_name = azurerm_virtual_network.vm.name
  address_prefixes     = ["10.42.1.0/24"]
  service_endpoints    = ["Microsoft.Sql"]
}

# ---------------------------------------------------------------------------
# Network security group: SSH only from admin_cidr, SQL only to Azure SQL
# public endpoint (Internet), HTTPS outbound. No database port exposed to the
# internet from the VM subnet perspective; the Azure SQL firewall pins the VM
# public IP separately.
# ---------------------------------------------------------------------------
resource "azurerm_network_security_group" "app" {
  provider            = azurerm.vm
  name                = local.vm.nsg
  location            = azurerm_resource_group.vm.location
  resource_group_name = azurerm_resource_group.vm.name
  tags                = local.tags

  security_rule {
    name                       = "Allow-SSH-From-Admin"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = tostring(local.required_ports.ssh)
    source_address_prefix      = var.admin_cidr
    destination_address_prefix = "*"
    description                = "SSH restricted to the operator CIDR."
  }

  security_rule {
    name                       = "Deny-Inbound-Internet"
    priority                   = 4000
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "Internet"
    destination_address_prefix = "*"
    description                = "Deny any other inbound internet traffic."
  }

  security_rule {
    name                       = "Allow-Outbound-HTTPS"
    priority                   = 100
    direction                  = "Outbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "443"
    source_address_prefix      = "*"
    destination_address_prefix = "Internet"
    description                = "Telegram Bot API and Azure REST APIs."
  }

  security_rule {
    name                       = "Allow-Outbound-AzureSQL"
    priority                   = 110
    direction                  = "Outbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = tostring(local.required_ports.sql)
    source_address_prefix      = "*"
    destination_address_prefix = "Internet"
    description                = "Azure SQL public endpoint tcp/1433."
  }

  security_rule {
    name                       = "Allow-Outbound-Supabase"
    priority                   = 120
    direction                  = "Outbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = tostring(var.supabase_db_port)
    source_address_prefix      = "*"
    destination_address_prefix = "Internet"
    description                = "Supabase PostgreSQL endpoint."
  }
}

resource "azurerm_subnet_network_security_group_association" "app" {
  provider                  = azurerm.vm
  subnet_id                 = azurerm_subnet.app.id
  network_security_group_id = azurerm_network_security_group.app.id
}

# ---------------------------------------------------------------------------
# Public IP (static) so each Azure SQL firewall can be pinned to this VM only.
# ---------------------------------------------------------------------------
resource "azurerm_public_ip" "vm" {
  provider            = azurerm.vm
  name                = local.vm.public_ip
  location            = azurerm_resource_group.vm.location
  resource_group_name = azurerm_resource_group.vm.name
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = local.tags
}

resource "azurerm_network_interface" "vm" {
  provider            = azurerm.vm
  name                = local.vm.nic
  location            = azurerm_resource_group.vm.location
  resource_group_name = azurerm_resource_group.vm.name
  tags                = local.tags

  ip_configuration {
    name                          = "ipconfig1"
    subnet_id                     = azurerm_subnet.app.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = var.enable_public_ip ? azurerm_public_ip.vm.id : null
  }
}
