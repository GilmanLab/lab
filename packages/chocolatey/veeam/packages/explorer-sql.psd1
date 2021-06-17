@{
    name      = 'veeam-explorer-sql'
    installer = @{
        scriptPath    = 'tools'
        installerPath = 'Explorers\VeeamExplorerforSQL.msi'
        installerType = 'msi'
        exitCodes     = @(0, 1638, 1641, 3010)
        flags         = '/qn /norestart'
        arguments     = @{
            ACCEPT_EULA                = '1'
            ACCEPT_THIRDPARTY_LICENSES = '1'
        }
    }
    manifest  = @{
        metadata = @{
            id                       = 'veeam-explorer-sql'
            title                    = 'Veeam Explorer for Microsoft SQL Server'
            version                  = '11.0.0.837'
            authors                  = 'Veeam'
            owners                   = 'Gilman Lab'
            summary                  = 'Installs the Veeam Explorer for Microsoft SQL Server'
            description              = 'Veeam Backup & Replication is a backup solution developed for VMware vSphere and Microsoft Hyper-V virtual environments. Veeam Backup & Replication provides a set of features for performing data protection and disaster recovery tasks.'
            projectUrl               = 'http://www.veeam.com/'
            packageSourceUrl         = 'https://dev.azure.com/GilmanLab/Lab/_git/Packages'
            tags                     = 'veeam backup replication explorer microsoft sql'
            copyright                = '2021 Veeam'
            licenseUrl               = 'https://www.veeam.com/eula.html'
            requireLicenseAcceptance = 'false'
            dependencies             = @(
                @{
                    id      = 'veeam-iso'
                    version = '[11.0.0.837]'
                }
            )
        }
        files    = @(
            @{
                src    = 'tools\**'
                target = 'tools'
            }
        )
    }
}