$tok='eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1dWlkIjoiNThkNTdiMTUtOGVjZC00NzY1LWI1NmUtMmRjOWViYmUzMDEwIiwidXNlcm5hbWUiOm51bGwsInJvbGUiOiJBUEkiLCJpYXQiOjE3NzU0OTM4NjksImV4cCI6MTA0MTU0MDc0Njl9.ZJqnJQP2FOCbidc2gI17YErTc4bGgL-6nzHx3_RxyMo'
$h=@{Authorization="Bearer $tok"}
$p='https://pp.komaru.sh'

Write-Host '=== PROFILES ==='
$profiles=(Invoke-RestMethod "$p/api/config-profiles" -Headers $h).response.configProfiles
foreach($pr in $profiles){
  $inbs=($pr.inbounds | ForEach-Object { "$($_.tag)[$($_.uuid)]" }) -join '; '
  "{0}  uuid={1}`n   inbounds: {2}" -f $pr.name,$pr.uuid,$inbs
}

Write-Host "`n=== NODES ==="
$nodes=(Invoke-RestMethod "$p/api/nodes" -Headers $h).response
foreach($n in $nodes){
  $sq=($n.activeInternalSquads | ForEach-Object { $_.uuid }) -join ','
  "{0}  uuid={1}`n   profile={2}  squads={3}" -f $n.name,$n.uuid,$n.activeConfigProfileUuid,$sq
}

Write-Host "`n=== SQUADS ==="
$squads=(Invoke-RestMethod "$p/api/internal-squads" -Headers $h).response.internalSquads
foreach($s in $squads){
  $ib=($s.inbounds | ForEach-Object { $_.uuid }) -join ','
  "{0}  uuid={1}  inbounds={2}" -f $s.name,$s.uuid,$ib
}
