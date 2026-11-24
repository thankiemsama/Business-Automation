$base = 'http://127.0.0.1:8082'
$secret = 'e2e2-proxy-secret-abcdef0123456789'
function Call($Method, $Url, $Body, $Ip, $Token = '') {
    $h = @{ 'X-Nexa-Client-IP' = $Ip; 'X-Nexa-Proxy-Secret' = $secret; 'Content-Type' = 'application/json' }
    if ($Token) { $h['Authorization'] = "Bearer $Token" }
    $p = @{ Method = $Method; Uri = $Url; Headers = $h; UseBasicParsing = $true }
    if ($null -ne $Body) { $p.Body = (ConvertTo-Json $Body -Compress) }
    try { $r = Invoke-WebRequest @p; return [pscustomobject]@{ Status=[int]$r.StatusCode; Body=($r.Content|ConvertFrom-Json); Headers=$r.Headers } }
    catch { $resp=$_.Exception.Response; if($resp){ $rd=[IO.StreamReader]::new($resp.GetResponseStream()); $t=$rd.ReadToEnd(); try{$j=$t|ConvertFrom-Json}catch{$j=@{raw=$t}}; return [pscustomobject]@{Status=[int]$resp.StatusCode; Body=$j; Headers=$resp.Headers} } else { return [pscustomobject]@{Status=-1; Body=@{raw=$_.Exception.Message}; Headers=@{}} } }
}
function LastLink($pattern) {
    $lines = Get-Content 'D:\GOLANG\data\e2e2.err.log' -Encoding UTF8
    $hits = $lines | Where-Object { $_ -match $pattern } | Select-Object -Last 1
    if (-not $hits) { return $null }
    return [System.Uri]::UnescapeDataString($hits.Split('#')[-1].TrimEnd('"'))
}
$pwd = 'password-123456'

# LOCKOUT reason (clean: register first, fresh bucket)
Call POST "$base/api/v1/auth/register" @{name='LK';email='lk@e2e2.test';password=$pwd} '10.11.1.1' | Out-Null
for ($i=1;$i -le 5;$i++){ Call POST "$base/api/v1/auth/login" @{email='lk@e2e2.test';password='wrong-wrong-1234'} '10.11.1.2' | Out-Null }
$r = Call POST "$base/api/v1/auth/login" @{email='lk@e2e2.test';password=$pwd} '10.11.1.2'
Write-Host ("LOCKOUT: status={0} reason={1} msg={2}" -f $r.Status, $r.Body.reason, $r.Body.message)

# INVITE flow
Call POST "$base/api/v1/auth/register" @{name='InvO';email='invo@e2e2.test';password=$pwd} '10.11.2.1' | Out-Null
$tok = (Call POST "$base/api/v1/auth/login" @{email='invo@e2e2.test';password=$pwd} '10.11.2.1').Body.data.access_token
$r = Call POST "$base/api/v1/team/invite" @{name='InvAgent';email='invagent@e2e2.test'} '10.11.2.2' $tok
Write-Host ("INVITE: status={0} msg={1}" -f $r.Status, $r.Body.message)
Start-Sleep -Seconds 1
$invTok = LastLink 'account/setup#'
Write-Host ("INVITE link token = {0}" -f ($invTok | Select-Object -First 30))
# unknown-email invite validation
$r2 = Call POST "$base/api/v1/team/invite" @{name='Bad';email='not-an-email'} '10.11.2.2' $tok
Write-Host ("INVITE malformed email: status={0} msg={1}" -f $r2.Status, $r2.Body.message)

# FORGOT: expired-link path is covered by Go clock test; here just confirm link extraction works after the dev-sink fix
Call POST "$base/api/v1/auth/register" @{name='Fg';email='fg@e2e2.test';password=$pwd} '10.11.3.1' | Out-Null
$r3 = Call POST "$base/api/v1/auth/forgot" @{email='fg@e2e2.test'} '10.11.3.1'
Start-Sleep -Seconds 1
$fgTok = LastLink 'account/setup#'
Write-Host ("FORGOT: status={0} extracted-token-len={1}" -f $r3.Status, ($fgTok.Length))
# redeem with valid token -> new pw
$r4 = Call POST "$base/api/v1/auth/redeem" @{token=$fgTok; password='reset-12345678'} '10.11.3.2'
Write-Host ("REDEEM valid: status={0} msg={1}" -f $r4.Status, $r4.Body.message)
$r5 = Call POST "$base/api/v1/auth/redeem" @{token=$fgTok; password='replay-12345678'} '10.11.3.2'
Write-Host ("REDEEM replay: status={0} msg={1}" -f $r5.Status, $r5.Body.message)
