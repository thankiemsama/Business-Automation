# E2E auth pentest v2 — strict IP bucket isolation + MFA lifecycle
$base = 'http://127.0.0.1:8082'
$secret = 'e2e2-proxy-secret-abcdef0123456789'
$dbFile = 'D:\GOLANG\data\e2e2.json'
$results = New-Object System.Collections.Generic.List[object]

function Call {
    param([string]$Method, [string]$Url, $Body, [string]$Ip, [string]$Token = '', [string]$ContentType = 'application/json')
    $h = @{ 'X-Nexa-Client-IP' = $Ip; 'X-Nexa-Proxy-Secret' = $secret }
    if ($Token) { $h['Authorization'] = "Bearer $Token" }
    if ($ContentType) { $h['Content-Type'] = $ContentType }
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
        } else {
            [pscustomobject]@{ Status = -1; Headers = @{}; Body = @{ raw = $_.Exception.Message } }
        }
    }
}

function CallRaw {
    param([string]$Method, [string]$Url, [string]$Text, [string]$Ip, [string]$ContentType = 'application/json')
    $h = @{ 'X-Nexa-Client-IP' = $Ip; 'X-Nexa-Proxy-Secret' = $secret }
    if ($ContentType) { $h['Content-Type'] = $ContentType }
    try {
        $r = Invoke-WebRequest -Method $Method -Uri $Url -Headers $h -Body $Text -UseBasicParsing
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
    $pass = ($Expected -eq $Actual) -or ($Expected -is [array] -and $Expected -contains $Actual)
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
    for ($b = 0; $b + 7 -lt $bits.Length; $b += 8) {
        $out.Add([byte][Convert]::ToInt32($bits.Substring($b, 8), 2))
    }
    return $out.ToArray()
}
# RFC-6238 TOTP reference implementation (SHA-1, 30s, 6 digits)
function TOTP($secretB32, $step) {
    $raw = Base32Decode $secretB32
    $block = New-Object byte[] 8
    [BitConverter]::GetBytes([long]$step) | Copy-To $block
    $hmac = [System.Security.Cryptography.HMACSHA1]::Create()
    $hmac.Key = $raw
    $d = $hmac.ComputeHash($block)
    $o = $d[$d.Length - 1] -band 15
    $v = ([uint32]$d[$o] -shl 24 -bor [uint32]$d[$o+1] -shl 16 -bor [uint32]$d[$o+2] -shl 8 -bor [uint32]$d[$o+3]) -band 0x7fffffff
    return ('{0:D6}' -f ($v % 1000000))
}

# Extract the "tenant.secret" reset token from the file DB (last reset link record)
function LastResetToken() {
    $db = Get-Content $dbFile -Raw | ConvertFrom-Json
    $records = $db.records.PSObject.Properties | Where-Object { $_.Value.Kind -eq 'account_links' -and $_.Value.Subject -eq 'reset' -and $_.Value.Enabled } | Sort-Object { $_.Value.CreatedAt } | Select-Object -Last 1
    if (-not $records) { return $null }
    $tenant = $records.Value.TenantID
    # record ID is sha256(token); recover the token: the link fragment is "tenant.token",
    # and the record's ID = tokenHash(token) = sha256hex(token). We stored only the hash,
    # so we cannot reverse it — but the mail body (dev-sink err.log) carries the full link.
    return $null
}

function LastResetLinkFromErrLog() {
    $lines = Get-Content 'D:\GOLANG\data\e2e2.err.log' -Encoding UTF8
    $hits = $lines | Where-Object { $_ -match 'account/setup#' } | Select-Object -Last 1
    if (-not $hits) { return $null }
    $frag = $hits.Split('#')[-1].TrimEnd('"')
    return [System.Uri]::UnescapeDataString($frag)
}

# Buckets — each section gets its own IP to stay under the 20/min limiter
$ipReg  = '10.0.1.1'
$ipLog1 = '10.0.2.1'   # main user: login + password + logout (fewer than 20)
$ipLog2 = '10.0.2.2'   # lockout user: 5 fails + 1 correct + 1 ghost
$ipLog3 = '10.0.2.3'   # long-pw user
$ipTeam = '10.0.3.1'
$ipRed  = '10.0.4.1'
$ipRl   = '10.0.5.1'
$ipHdr  = '10.0.6.1'
$ipMfa  = '10.0.7.1'

$pwd = 'password-123456'

# ===== A. REGISTER =====
$r = Call POST "$base/api/v1/auth/register" @{ name='Main User'; email='main@e2e2.test'; password=$pwd } $ipReg
Check 'R1 register valid' 'Register' 201 $r.Status
$data = $r.Body.data
$leak = @('password_hash','salt','totp_secret','recovery_hashes','password') | Where-Object { $data.PSObject.Properties.Name -contains $_ }
Check 'R1b no secrets in payload' 'Register' '' ($leak -join ',') 'public fields only'
Check 'R1c role owner' 'Register' 'owner' $data.role
Check 'R1d email_verification_pending=false (auto-confirm)' 'Register' $false $data.email_verification_pending

$r = Call POST "$base/api/v1/auth/register" @{ name='Dup'; email='MAIN@E2E2.TEST'; password=$pwd } $ipReg
Check 'R2 duplicate email case-variant' 'Register' 409 $r.Status "msg=$($r.Body.message)"
$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r3@e2e2.test'; password='short-11ch' } $ipReg
Check 'R3 pw 11 chars' 'Register' 400 $r.Status
$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r4@e2e2.test'; password=('p'*129) } $ipReg
Check 'R4 pw 129 chars' 'Register' 400 $r.Status
$r = Call POST "$base/api/v1/auth/register" @{ email='r5@e2e2.test'; password=$pwd } $ipReg
Check 'R5 empty name' 'Register' 400 $r.Status
$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='not-an-email'; password=$pwd } $ipReg
Check 'R6 malformed email' 'Register' 400 $r.Status
$r = CallRaw POST "$base/api/v1/auth/register" '{"name":"X","email":"A <a@e2e2.test>","password":"password-123456"}' $ipReg
Check 'R7 angle-bracket email' 'Register' 400 $r.Status
$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r8@e2e2.test'; password=$pwd; role='admin' } $ipReg
Check 'R8 role injection' 'Register' 400 $r.Status
$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r9@e2e2.test'; password=$pwd; admin=$true } $ipReg
Check 'R9 admin field injection' 'Register' 400 $r.Status
$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r10@e2e2.test'; password=$pwd; industry='unknown' } $ipReg
Check 'R10 bogus industry' 'Register' 400 $r.Status
$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r11@e2e2.test'; password=$pwd; industry='ecommerce' } $ipReg
Check 'R11 valid industry' 'Register' 201 $r.Status
$r = CallRaw POST "$base/api/v1/auth/register" '[1,2,3]' $ipReg
Check 'R12 JSON array body' 'Register' 400 $r.Status
$r = CallRaw POST "$base/api/v1/auth/register" '{"name":"X","email":"r13@e2e2.test","password":"password-123456"}' $ipReg 'text/plain'
Check 'R13 text/plain content-type' 'Register' 415 $r.Status

# ===== B. LOGIN =====
$r = Call POST "$base/api/v1/auth/login" @{ email='main@e2e2.test'; password=$pwd } $ipLog1
Check 'L1 login valid' 'Login' 200 $r.Status
$mainToken = $r.Body.data.access_token
Check 'L1b token_type' 'Login' 'Bearer' $r.Body.data.token_type
Check 'L1c expires_in' 'Login' 86400 $r.Body.data.expires_in

$r = Call POST "$base/api/v1/auth/login" @{ email='main@e2e2.test'; password='wrong-password-1' } $ipLog1
Check 'L2 wrong password' 'Login' 401 $r.Status
$msgL2 = $r.Body.message

$r = Call POST "$base/api/v1/auth/login" @{ email='ghost@e2e2.test'; password='password-123456' } $ipLog1
Check 'L3 unknown email (enum-safe)' 'Login' 401 $r.Status
Check 'L3b identical message' 'Login' $msgL2 $r.Body.message 'anti-enumeration'

$r = Call POST "$base/api/v1/auth/login" @{ email='MAIN@E2E2.TEST'; password=$pwd } $ipLog1
Check 'L4 case-insensitive email' 'Login' 200 $r.Status
$mainToken2 = $r.Body.data.access_token

$r = Call GET "$base/api/v1/auth/login" $null $ipLog1
Check 'L5 GET not allowed' 'Login' 405 $r.Status
$r = CallRaw POST "$base/api/v1/auth/login" '{}' $ipLog1
Check 'L6 empty body' 'Login' 401 $r.Status

# lockout
Call POST "$base/api/v1/auth/register" @{ name='Lock'; email='lock@e2e2.test'; password=$pwd } $ipLog2 | Out-Null
$lastFail = 0
for ($i = 1; $i -le 5; $i++) {
    $r = Call POST "$base/api/v1/auth/login" @{ email='lock@e2e2.test'; password='totally-wrong-1' } $ipLog2
    $lastFail = $r.Status
}
Check 'L8 5th wrong attempt' 'Lockout' 401 $lastFail
$r = Call POST "$base/api/v1/auth/login" @{ email='lock@e2e2.test'; password=$pwd } $ipLog2
Check 'L8b correct pw while locked' 'Lockout' 401 $r.Status "msg=$($r.Body.message)"
Check 'L8c reason account_locked' 'Lockout' 'account_locked' $r.Body.reason
$r = Call POST "$base/api/v1/auth/login" @{ email='ghost@e2e2.test'; password='nope-12345' } $ipLog2
Check 'L8d unknown email no lockout leak' 'Lockout' 401 $r.Status

# long password at login
Call POST "$base/api/v1/auth/register" @{ name='Long'; email='long@e2e2.test'; password=$pwd } $ipLog3 | Out-Null
$r = Call POST "$base/api/v1/auth/login" @{ email='long@e2e2.test'; password=('p'*129) } $ipLog3
Check 'L7 129-char password' 'Login' 401 $r.Status

# disabled agent
$regT = Call POST "$base/api/v1/auth/register" @{ name='Team Owner'; email='team@e2e2.test'; password=$pwd } $ipTeam
$teamToken = $regT.Body.data.access_token
$r = Call POST "$base/api/v1/team" @{ name='Agent'; email='agent@e2e2.test'; password='agent-pass-12345' } $ipTeam $teamToken
Check 'L9 owner creates agent' 'Team' 201 $r.Status "msg=$($r.Body.message)"
$agentId = $r.Body.data.id
$r = Call PUT "$base/api/v1/team/$agentId" @{ name='Agent'; email='agent@e2e2.test'; disabled=$true } $ipTeam $teamToken
Check 'L9b owner disables agent' 'Team' 200 $r.Status "msg=$($r.Body.message)"
$r = Call POST "$base/api/v1/auth/login" @{ email='agent@e2e2.test'; password='agent-pass-12345' } $ipTeam
Check 'L9c disabled agent login' 'Team' 401 $r.Status
# agent cannot create agents
$agentLogin = Call POST "$base/api/v1/auth/login" @{ email='agent@e2e2.test'; password='agent-pass-12345' } $ipTeam
# agent was disabled above so this is 401; use a fresh agent
Call PUT "$base/api/v1/team/$agentId" @{ name='Agent'; email='agent@e2e2.test'; disabled=$false } $ipTeam $teamToken | Out-Null
$agentToken = (Call POST "$base/api/v1/auth/login" @{ email='agent@e2e2.test'; password='agent-pass-12345' } $ipTeam).Body.data.access_token
$r = Call POST "$base/api/v1/team" @{ name='X'; email='x@e2e2.test'; password='x-password-12345' } $ipTeam $agentToken
Check 'L9d agent cannot create agent' 'Team' 403 $r.Status "msg=$($r.Body.message)"

# cross-tenant isolation
$reg2 = Call POST "$base/api/v1/auth/register" @{ name='Other Tenant'; email='team2@e2e2.test'; password=$pwd } $ipTeam
$team2Token = $reg2.Body.data.access_token
$r = Call PUT "$base/api/v1/team/$agentId" @{ name='H'; email='h@e2e2.test' } $ipTeam $team2Token
Check 'L9e cross-tenant team update' 'Team' 404 $r.Status

# ===== C. PASSWORD CHANGE =====
$r = Call POST "$base/api/v1/auth/password" @{ old_password='nope-nope-12345'; password='new-password-1234' } $ipLog1 $mainToken2
Check 'P1 wrong old pw' 'Password' 400 $r.Status
$r = Call POST "$base/api/v1/auth/password" @{ old_password='short'; password='new-password-1234' } $ipLog1 $mainToken2
Check 'P2 11-char new pw' 'Password' 400 $r.Status
$r = Call POST "$base/api/v1/auth/password" @{ old_password=$pwd; password=('p'*129) } $ipLog1 $mainToken2
Check 'P3 129-char new pw' 'Password' 400 $r.Status
$r = Call POST "$base/api/v1/auth/password" @{ old_password=$pwd; password='new-password-1234' } $ipLog1 $mainToken2
Check 'P4 valid change' 'Password' 200 $r.Status
$r = Call GET "$base/api/v1/auth/me" $null $ipLog1 $mainToken2
Check 'P5 old session revoked' 'Password' 401 $r.Status

# ===== D. LOGOUT =====
$newLogin = Call POST "$base/api/v1/auth/login" @{ email='main@e2e2.test'; password='new-password-1234' } $ipLog1
$mainToken3 = $newLogin.Body.data.access_token
$r = Call POST "$base/api/v1/auth/logout" $null $ipLog1 $mainToken3
Check 'O1 logout' 'Logout' 200 $r.Status
$r = Call GET "$base/api/v1/auth/me" $null $ipLog1 $mainToken3
Check 'O2 token dead after logout' 'Logout' 401 $r.Status

# ===== E. FORGOT / REDEEM =====
$regRed = Call POST "$base/api/v1/auth/register" @{ name='Redeemer'; email='redeemer@e2e2.test'; password=$pwd } $ipRed
$redLogin = Call POST "$base/api/v1/auth/login" @{ email='redeemer@e2e2.test'; password=$pwd } $ipRed
$redToken = $redLogin.Body.data.access_token
$r = Call POST "$base/api/v1/auth/forgot" @{ email='redeemer@e2e2.test' } $ipRed
Check 'F1 forgot known email' 'Forgot' 200 $r.Status
$r = Call POST "$base/api/v1/auth/forgot" @{ email='nobody@e2e2.test' } $ipRed
Check 'F2 forgot unknown email' 'Forgot' 200 $r.Status "enum-safe"
$r = Call POST "$base/api/v1/auth/redeem" @{ token=''; password='password-123456' } $ipRed
Check 'F3 empty token' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token='no-dot-at-all'; password='password-123456' } $ipRed
Check 'F4 no dot' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token='tenant.'; password='password-123456' } $ipRed
Check 'F5 empty secret' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token='t.'+('b'*48); password='password-123456' } $ipRed
Check 'F6 random token' 'Forgot' 400 $r.Status

Start-Sleep -Seconds 1
$token = LastResetLinkFromErrLog
Check 'F7 extract reset link from dev-sink log' 'Forgot' $true ($null -ne $token) "token=$($token | Select-Object -First 20)..."
$r = Call POST "$base/api/v1/auth/redeem" @{ token=$token; password='short' } $ipRed
Check 'F8 valid token + short pw' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token=$token; password='recovered-12345' } $ipRed
Check 'F9 redeem valid' 'Forgot' 200 $r.Status "msg=$($r.Body.message)"
$r = Call GET "$base/api/v1/auth/me" $null $ipRed $redToken
Check 'F10 sessions revoked after redeem' 'Forgot' 401 $r.Status
$r = Call POST "$base/api/v1/auth/login" @{ email='redeemer@e2e2.test'; password='recovered-12345' } $ipRed
Check 'F11 login new pw' 'Forgot' 200 $r.Status
$r = Call POST "$base/api/v1/auth/login" @{ email='redeemer@e2e2.test'; password=$pwd } $ipRed
Check 'F12 old pw rejected' 'Forgot' 401 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token=$token; password='reused-12345678' } $ipRed
Check 'F13 replay token' 'Forgot' 400 $r.Status "single-use"

# ===== F. VERIFY / CONFIRM =====
$r = Call POST "$base/api/v1/auth/verify" @{ email='nobody@e2e2.test' } $ipRed
Check 'V1 resend verify unknown' 'Verify' 200 $r.Status "enum-safe"
$r = Call POST "$base/api/v1/auth/confirm" @{ token='garbage' } $ipRed
Check 'V3 confirm garbage' 'Verify' 400 $r.Status

# ===== G. RATE LIMIT =====
$rl401 = 0; $rl429 = 0; $lastCode = 0
for ($i = 1; $i -le 21; $i++) {
    $r = Call POST "$base/api/v1/auth/login" @{ email='rl@e2e2.test'; password='wrong-password-1' } $ipRl
    $lastCode = $r.Status
    if ($r.Status -eq 401) { $rl401++ } elseif ($r.Status -eq 429) { $rl429++ }
}
Check 'RL1 first 20 -> 401' 'RateLimit' 20 $rl401 "got=$rl401"
Check 'RL2 21st -> 429' 'RateLimit' 1 $rl429 "last=$lastCode"
$r = Call POST "$base/api/v1/auth/login" @{ email='rl2@e2e2.test'; password='wrong-password-1' } $ipRl
Check 'RL3 same bucket still 429' 'RateLimit' 429 $r.Status

# ===== H. SECURITY HEADERS =====
$r = Call GET "$base/api/v1/auth/me" $null $ipHdr
Check 'H1 X-Content-Type-Options' 'Headers' 'nosniff' $r.Headers.'X-Content-Type-Options'
Check 'H2 Cache-Control' 'Headers' 'no-store' $r.Headers.'Cache-Control'
$r = Call GET "$base/api/v1/does-not-exist" $null $ipHdr
Check 'H3 unknown endpoint' 'Headers' 404 $r.Status

# ===== I. MFA LIFECYCLE =====
$mfaReg = Call POST "$base/api/v1/auth/register" @{ name='MFA User'; email='mfa@e2e2.test'; password=$pwd } $ipMfa
$mfaLogin = Call POST "$base/api/v1/auth/login" @{ email='mfa@e2e2.test'; password=$pwd } $ipMfa
$mfaToken = $mfaLogin.Body.data.access_token
$r = Call GET "$base/api/v1/account/mfa" $null $ipMfa $mfaToken
Check 'M0 mfa status' 'MFA' 200 $r.Status
Check 'M0b mfa not enabled' 'MFA' $false $r.Body.data.enabled
$r = Call POST "$base/api/v1/account/mfa" @{ action='setup'; password='wrong-password-1' } $ipMfa $mfaToken
Check 'M1 setup wrong pw' 'MFA' 403 $r.Status
$r = Call POST "$base/api/v1/account/mfa" @{ action='setup'; password=$pwd } $ipMfa $mfaToken
Check 'M2 setup ok' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$secret = $r.Body.data.secret
$r = Call POST "$base/api/v1/account/mfa" @{ action='enable'; code='000000' } $ipMfa $mfaToken
Check 'M3 enable wrong code' 'MFA' '401|409' $r.Status
$step = [Math]::Floor(([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()) / 30)
$r = Call POST "$base/api/v1/account/mfa" @{ action='enable'; code=(TOTP $secret $step) } $ipMfa $mfaToken
Check 'M4 enable valid TOTP' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$codes = $r.Body.data.recovery_codes
Check 'M4b 8 recovery codes' 'MFA' 8 $codes.Count
Check 'M4c mfa enabled' 'MFA' $true $r.Body.data.enabled
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa@e2e2.test'; password=$pwd } $ipMfa
Check 'M5 login without MFA' 'MFA' 409 $r.Status "msg=$($r.Body.message)"
$nextStep = [Math]::Floor(([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()) / 30)
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa@e2e2.test'; password=$pwd; code=(TOTP $secret $nextStep) } $ipMfa
Check 'M6 login with TOTP' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$replayStep = [Math]::Floor(([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()) / 30)
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa@e2e2.test'; password=$pwd; code=(TOTP $secret $replayStep) } $ipMfa
Check 'M7 replay TOTP code' 'MFA' 401 $r.Status "replay-proof"
$mfaToken2 = $r2Token = $null
# login again with recovery code
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa@e2e2.test'; password=$pwd; code=$codes[0] } $ipMfa
Check 'M8 recovery code login' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$liveToken = $r.Body.data.access_token
$r = Call POST "$base/api/v1/auth/login" @{ email='mfa@e2e2.test'; password=$pwd; code=$codes[0] } $ipMfa
Check 'M9 replay recovery code' 'MFA' 401 $r.Status "single-use"
# disable
$r = Call POST "$base/api/v1/account/mfa" @{ action='disable'; password=$pwd; code=$codes[1] } $ipMfa $liveToken
Check 'M10 disable with pw+code' 'MFA' 200 $r.Status "msg=$($r.Body.message)"
$r = Call POST "$base/api/v1/account/mfa" @{ action='disable'; code=$codes[2] } $ipMfa $liveToken
Check 'M11 disable without pw' 'MFA' 403 $r.Status "password gate holds"

# sessions list + revoke
$sl = Call GET "$base/api/v1/account/sessions" $null $ipMfa $liveToken
Check 'S1 sessions list' 'MFA-Sessions' 200 $sl.Status "msg=$($sl.Body.message)"
$cur = $sl.Body.data | Where-Object { $_.current -eq $true } | Select-Object -First 1
$r = Call POST "$base/api/v1/account/sessions/revoke" @{ id=$cur.id } $ipMfa $liveToken
Check 'S2 revoke current session' 'MFA-Sessions' 200 $r.Status "msg=$($r.Body.message)"
$r = Call GET "$base/api/v1/auth/me" $null $ipMfa $liveToken
Check 'S3 revoked token dead' 'MFA-Sessions' 401 $r.Status

# ===== PERSIST =====
$results | ForEach-Object {
    $s = if ($_.Pass) { 'PASS' } else { 'FAIL' }
    [string]::Join('|', @($s, $_.Section, $_.Test, $_.Expected, $_.Actual, $_.Note))
} | Set-Content 'D:\GOLANG\data\e2e2_results.txt' -Encoding utf8
$fail = @($results | Where-Object { -not $_.Pass }).Count
Write-Host ("TOTAL={0} PASS={1} FAIL={2}" -f $results.Count, ($results.Count - $fail), $fail)
if ($fail -gt 0) { Write-Host '--- FAILED CASES ---'; $results | Where-Object { -not $_.Pass } | ForEach-Object { Write-Host ("FAIL: {0} :: {1} expected={2} actual={3} {4}" -f $_.Section, $_.Test, $_.Expected, $_.Actual, $_.Note) } }
