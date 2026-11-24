# Focused v3: lockout reason, team, headers, MFA lifecycle (correct TOTP)
$base = 'http://127.0.0.1:8082'
$secret = 'e2e2-proxy-secret-abcdef0123456789'
$results = New-Object System.Collections.Generic.List[object]

function Call {
    param([string]$Method, [string]$Url, $Body, [string]$Ip, [string]$Token = '')
    $h = @{ 'X-Nexa-Client-IP' = $Ip; 'X-Nexa-Proxy-Secret' = $secret; 'Content-Type' = 'application/json' }
    if ($Token) { $h['Authorization'] = "Bearer $Token" }
    $params = @{ Method = $Method; Uri = $Url; Headers = $h; UseBasicParsing = $true }
    if ($null -ne $Body) { $params.Body = (ConvertTo-Json $Body -Compress) }
    try {
        $r = Invoke-WebRequest @params
        $j = $r.Content | ConvertFrom-Json
        [pscustomobject]@{ Status = [int]$r.StatusCode; Headers = $r.Headers; Body = $j }
    } catch {
        $resp = $_.Exception.Response
        if ($resp) {
            $reader = [System.IO.StreamReader]::new($resp.GetResponseStream())
            $text = $reader.ReadToEnd()
            try { $j = $text | ConvertFrom-Json } catch { $j = @{ raw = $text } }
            [pscustomobject]@{ Status = [int]$resp.StatusCode; Headers = $resp.Headers; Body = $j }
        } else { [pscustomobject]@{ Status = -1; Headers = @{}; Body = @{ raw = $_.Exception.Message } } }
    }
}

function Check($Name, $Section, $Expected, $Actual, $Note = '') {
    $pass = if ($Expected -is [array]) { $Expected -contains $Actual } else { $Expected -eq $Actual }
    $results.Add([pscustomobject]@{ Section = $Section; Test = $Name; Expected = ($Expected -join '|'); Actual = $Actual; Pass = $pass; Note = $Note })
    $mark = if ($pass) { 'PASS' } else { 'FAIL' }
    Write-Host ("[{0}] {1} :: {2} (expect {3}, got {4})" -f $mark, $Section, $Name, ($Expected -join '|'), $Actual)
}

function Base32Decode([string]$s) {
    $alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
    $s = $s.ToUpper().TrimEnd('=')
    $bits = ''
    foreach ($ch in $s.ToCharArray()) {
        $i = $alphabet.IndexOf($ch)
        if ($i -lt 0) { continue }
        $bits += [Convert]::ToString($i, 2).PadLeft(5, '0')
    }
    $out = [System.Collections.Generic.List[byte]]::new()
    for ($b = 0; $b + 8 -le $bits.Length; $b += 8) {
        $out.Add([byte][Convert]::ToInt32($bits.Substring($b, 8), 2))
    }
    return $out.ToArray()
}
function TOTP($secretB32, $step) {
    $raw = Base32Decode $secretB32
    $block = New-Object byte[] 8
    $sb = [BitConverter]::GetBytes([long]$step)
    $sb.CopyTo($block, 0)
    $hmac = [System.Security.Cryptography.HMACSHA1]::Create()
    $hmac.Key = $raw
    $d = $hmac.ComputeHash($block)
    $o = $d[$d.Length - 1] -band 15
    $v = ([uint32]$d[$o] -shl 24 -bor [uint32]$d[$o+1] -shl 16 -bor [uint32]$d[$o+2] -shl 8 -bor [uint32]$d[$o+3]) -band 0x7fffffff
    return ('{0:D6}' -f ($v % 1000000))
}
function NowStep() { [Math]::Floor([DateTimeOffset]::UtcNow.ToUnixTimeSeconds() / 30) }

$pwd = 'password-123456'
$ipL = '10.8.1.1'  # lockout
$ipT = '10.8.2.1'  # team
$ipM = '10.8.3.1'  # mfa
$ipH = '10.8.4.1'  # headers

# ---- LOCKOUT reason ----
Call POST "$base/api/v1/auth/register" @{ name='L3'; email='lock3@e2e2.test'; password=$pwd } $ipL | Out-Null
for ($i = 1; $i -le 5; $i++) { Call POST "$base/api/v1/auth/login" @{ email='lock3@e2e2.test'; password='totally-wrong-1' } $ipL | Out-Null }
$r = Call POST "$base/api/v1/auth/login" @{ email='lock3@e2e2.test'; password=$pwd } $ipL
Check 'L8c reason account_locked' 'Lockout' 'account_locked' $r.Body.reason "msg=$($r.Body.message)"

# ---- TEAM (proper login token) ----
Call POST "$base/api/v1/auth/register" @{ name='Owner3'; email='team3@e2e2.test'; password=$pwd } $ipT | Out-Null
$teamToken = (Call POST "$base/api/v1/auth/login" @{ email='team3@e2e2.test'; password=$pwd } $ipT).Body.data.access_token
$r = Call POST "$base/api/v1/team" @{ name='Agent'; email='agent3@e2e2.test'; password='agent-pass-12345' } $ipT $teamToken
Check 'L9 owner creates agent' 'Team' 201 $r.Status "msg=$($r.Body.message)"
$agentId = $r.Body.data.id
$r = Call PUT "$base/api/v1/team/$agentId" @{ name='Agent'; email='agent3@e2e2.test'; disabled=$true } $ipT $teamToken
Check 'L9b owner disables agent' 'Team' 200 $r.Status "msg=$($r.Body.message)"
$r = Call POST "$base/api/v1/auth/login" @{ email='agent3@e2e2.test'; password='agent-pass-12345' } $ipT
Check 'L9c disabled agent login' 'Team' 401 $r.Status

# ---- HEADERS ----
$r = Call GET "$base/api/v1/auth/me" $null $ipH
Check 'H1 X-Content-Type-Options' 'Headers' 'nosniff' $r.Headers.'X-Content-Type-Options'
Check 'H2 Cache-Control' 'Headers' 'no-store' $r.Headers.'Cache-Control'

# ---- MFA LIFECYCLE (correct TOTP) ----
Call POST "$base/api/v1/auth/register" @{ name='MFA3'; email='mfa3@e2e2.test'; password=$pwd } $ipM | Out-Null
$mfaToken = (Call POST "$base/api/v1/auth/login" @{ email='mfa3@e2e2.test'; password=$pwd } $ipM).Body.data.access_token
$r = Call POST "$base/api/v1/account/mfa" @{ action='setup'; password=$pwd } $ipM $mfaToken
Check 'M2 setup ok' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$secretB32 = $r.Body.data.secret
$st = NowStep
$r = Call POST "$base/api/v1/account/mfa" @{ action='enable'; code=(TOTP $secretB32 $st) } $ipM $mfaToken
Check 'M4 enable valid TOTP' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$codes = $r.Body.data.recovery_codes
Check 'M4b 8 recovery codes' 'MFA' 8 $codes.Count
Check 'M4c enabled true' 'MFA' $true $r.Body.data.enabled
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa3@e2e2.test'; password=$pwd } $ipM
Check 'M5 login without MFA' 'MFA' 409 $r.Status "msg=$($r.Body.message)"
$st2 = NowStep
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa3@e2e2.test'; password=$pwd; code=(TOTP $secretB32 $st2) } $ipM
Check 'M6 login with TOTP' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$st3 = NowStep
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa3@e2e2.test'; password=$pwd; code=(TOTP $secretB32 $st3) } $ipM
Check 'M7 replay TOTP rejected' 'MFA' 401 $r.Status "replay-proof"
$liveToken = (Call POST "$base/api/v1/auth/login" @{ email='mfa3@e2e2.test'; password=$pwd; code=$codes[0] } $ipM).Body.data.access_token
Check 'M8 recovery code login' 'MFA' $null $null "consumed codes[0]"
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa3@e2e2.test'; password=$pwd; code=$codes[0] } $ipM
Check 'M9 replay recovery code' 'MFA' 401 $r.Status "single-use"
$r = Call POST "$base/api/v1/account/mfa" @{ action='disable'; password=$pwd; code=$codes[1] } $ipM $liveToken
Check 'M10 disable with pw+code' 'MFA' 200 $r.Status "msg=$($r.Body.message)"

# ---- PERSIST ----
$results | ForEach-Object {
    $s = if ($_.Pass) { 'PASS' } else { 'FAIL' }
    [string]::Join('|', @($s, $_.Section, $_.Test, $_.Expected, $_.Actual, $_.Note))
} | Set-Content 'D:\GOLANG\data\e2e3_results.txt' -Encoding utf8
$fail = @($results | Where-Object { -not $_.Pass }).Count
Write-Host ("TOTAL={0} PASS={1} FAIL={2}" -f $results.Count, ($results.Count - $fail), $fail)
if ($fail -gt 0) { Write-Host '--- FAILED ---'; $results | Where-Object { -not $_.Pass } | ForEach-Object { Write-Host ("FAIL: {0} :: {1} expected={2} actual={3} {4}" -f $_.Section, $_.Test, $_.Expected, $_.Actual, $_.Note) } }
