$ErrorActionPreference = 'Stop'
$h = @{ Authorization = 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1dWlkIjoiNThkNTdiMTUtOGVjZC00NzY1LWI1NmUtMmRjOWViYmUzMDEwIiwidXNlcm5hbWUiOm51bGwsInJvbGUiOiJBUEkiLCJpYXQiOjE3NzU0OTM4NjksImV4cCI6MTA0MTU0MDc0Njl9.ZJqnJQP2FOCbidc2gI17YErTc4bGgL-6nzHx3_RxyMo'; 'Content-Type'='application/json' }
$base = 'https://pp.komaru.sh/api'

# 1. Build cloned config with mirage flow
$src = (Get-Content 'f:\xray\_profile_xray.json' -Raw | ConvertFrom-Json).response.config
# deep clone
$json = $src | ConvertTo-Json -Depth 30
$cfg = $json | ConvertFrom-Json
# rename inbound, switch flow
$cfg.inbounds[0].tag = 'VLESS_TCP_REALITY_MIRAGE_NL'
$cfg.inbounds[0].settings.flow = 'xtls-rprx-mirage'
# update routing rule that referenced old tag
foreach ($r in $cfg.routing.rules) {
    if ($r.PSObject.Properties.Name -contains 'inboundTag') {
        $tags = @($r.inboundTag)
        if ($tags -contains 'VLESS_TCP_REALITY_RU_WL_1') {
            $r.inboundTag = @('VLESS_TCP_REALITY_MIRAGE_NL')
        }
    }
}

$body = @{ name = 'xray-mirage'; config = $cfg } | ConvertTo-Json -Depth 30
Write-Host '=== POST config-profile ==='
$prof = Invoke-RestMethod -Method Post -Uri "$base/config-profiles" -Headers $h -Body $body
$prof | ConvertTo-Json -Depth 8 | Out-File 'f:\xray\_new_profile.json' -Encoding utf8
$profUuid = $prof.response.uuid
$inbUuid = $prof.response.inbounds[0].uuid
Write-Host "profileUuid=$profUuid  inboundUuid=$inbUuid"

# 2. Create squad
Write-Host '=== POST squad mirage ==='
$squad = Invoke-RestMethod -Method Post -Uri "$base/internal-squads" -Headers $h -Body (@{ name='mirage'; inbounds=@($inbUuid) } | ConvertTo-Json)
$squad | ConvertTo-Json -Depth 8 | Out-File 'f:\xray\_new_squad.json' -Encoding utf8
$squadUuid = $squad.response.uuid
Write-Host "squadUuid=$squadUuid"

# 3. Add admin & ruslan1 to squad
$users = @('be7a1864-8acb-44e4-af9d-a801e05b58d9','4c0f25ee-e73b-48ac-a6f0-cee96eb68066')
Write-Host '=== POST add-users to squad ==='
$add = Invoke-RestMethod -Method Post -Uri "$base/internal-squads/$squadUuid/bulk-actions/add-users" -Headers $h -Body (@{ users=$users } | ConvertTo-Json)
$add | ConvertTo-Json -Depth 6

# 4. Switch NL node
$nlUuid = '955e2b83-b61c-49a9-b03b-9178c8b55d9d'
Write-Host '=== PATCH NL node profile ==='
$nodePatch = @{ uuid=$nlUuid; configProfile=@{ activeConfigProfileUuid=$profUuid; activeInbounds=@($inbUuid) } } | ConvertTo-Json -Depth 5
$node = Invoke-RestMethod -Method Patch -Uri "$base/nodes" -Headers $h -Body $nodePatch
$node | ConvertTo-Json -Depth 6 | Out-File 'f:\xray\_node_patched.json' -Encoding utf8
Write-Host 'DONE'
