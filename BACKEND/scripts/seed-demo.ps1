#Requires -Version 5.1
<#
.SYNOPSIS
    Seed du lieu demo co dinh theo mau nganh E-commerce (Nexa Business Automation).

.DESCRIPTION
    Chay khi backend dang hoat dong (local hoac Render). Du lieu deterministic:
    email, ten, noi dung luon giong nhau o moi lan chay. Chay nhieu lan khong
    tao trung: tai khoan / tai nguyen da ton tai se duoc bo qua (co canh bao).

    Hai che do:
      - direct (mac dinh): goi thurong API cua Go (http://127.0.0.1:8080) voi
        Bearer token. Dung khi chay API local hoac Docker.
      - bff: goi thurong /api/platform cua Next.js (https://ten-mien-render.com)
        voi cookie HttpOnly. BATT BUOC voi instance Render vi API Go chi nghe
        trong container; BFF proxy vao loopback va co bien Origin dung.

    Luu y: tai khoan demo la co dinh -> chuan bi dung trong mo i pitch / demo.
    Neu chay tren public instance, ban chinh -Password truong hoc.

.PARAMETER Mode
    "direct" (mac dinh) hoac "bff".

.PARAMETER BaseUrl
    direct: URL goc cua Go API, mac dinh http://127.0.0.1:8080.
    bff: URL goc cua web (Next.js), mac dinh https://nexa-web-55cq.onrender.com.

.PARAMETER Password
    Mat khau chung cho owner va agent. Mac dinh: NexaDemo!2026
#>
param(
    [ValidateSet("direct", "bff")][string]$Mode = "direct",
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [string]$Password = "NexaDemo!2026"
)

$ErrorActionPreference = "Stop"
$base = $BaseUrl.TrimEnd("/")
if ($Mode -eq "bff") {
    if ($base -like "*127.0.0.1*" -or $base -like "http://localhost*") {
        Write-Host "Loi: mode bff can URL web co the (https), vi du -BaseUrl https://nexa-web-55cq.onrender.com" -ForegroundColor Red
        exit 1
    }
    if ($BaseUrl -eq "http://127.0.0.1:8080") {
        $base = "https://nexa-web-55cq.onrender.com"
        Write-Host "Chuan: mode bff khong co -BaseUrl -> dung $base (dung -BaseUrl de ghi che)" -ForegroundColor DarkYellow
    }
}
$api = if ($Mode -eq "bff") { "$base/api/platform" } else { "$base/api/v1" }

$headers = @{}
$cookieJar = @{}
if ($Mode -eq "bff") {
    $u = [uri]$base
    $headers["Origin"] = "$($u.Scheme)://$($u.Authority)"
}

function Get-CookieHeader {
    if ($cookieJar.Count -eq 0) { return $null }
    $pairs = foreach ($k in $cookieJar.Keys) { $k + "=" + $cookieJar[$k] }
    return ($pairs -join "; ")
}
function Capture-SetCookie($response) {
    $raw = $response.Headers["Set-Cookie"]
    if (-not $raw) { return }
    foreach ($c in @($raw)) {
        $pair = $c.Split(";")[0]
        $i = $pair.IndexOf("=")
        if ($i -le 0) { continue }
        $name = $pair.Substring(0, $i).Trim()
        $val = $pair.Substring($i + 1)
        if ($val -eq "") { $cookieJar.Remove($name) } else { $cookieJar[$name] = $val }
    }
}

function Call-Api([string]$Method, [string]$Path, $Body) {
    $reqHeaders = @{}
    foreach ($k in $headers.Keys) { $reqHeaders[$k] = $headers[$k] }
    $cookie = Get-CookieHeader
    if ($cookie) { $reqHeaders["Cookie"] = $cookie }
    $argsForRequest = @{
        Method    = $Method
        Uri       = "$api$Path"
        Headers   = $reqHeaders
        ContentType = "application/json; charset=utf-8"
    }
    if ($null -ne $Body) {
        $payload = $Body | ConvertTo-Json -Depth 10 -Compress
        $argsForRequest.Body = [System.Text.Encoding]::UTF8.GetBytes($payload)
    }
    $response = $null
    try {
        $response = Invoke-WebRequest @argsForRequest -UseBasicParsing
    } catch {
        $status = 0; $bodyText = $_.Exception.Message
        if ($_.Exception.Response) {
            $status = [int]$_.Exception.Response.StatusCode
            $reader = [System.IO.StreamReader]::new($_.Exception.Response.GetResponseStream())
            $bodyText = $reader.ReadToEnd()
        }
        throw "API loi [$status] $Method $Path -> $bodyText"
    }
    Capture-SetCookie $response
    $json = $response.Content | ConvertFrom-Json
    if ($json.data) { return $json.data } else { return $json }
}

function Log([string]$Msg) { Write-Host "    $Msg" }
function Note([string]$Msg) { Write-Host "    [SKIP] $Msg" -ForegroundColor DarkYellow }
function Warn([string]$Msg) { Write-Host "    [WARN] $Msg" -ForegroundColor DarkYellow }

function Get-List([string]$Path) {
    $raw = $null
    try { $raw = Call-Api GET $Path $null } catch { return @() }
    # Resource lists are paginated: { items, total, page, limit }. Team and
    # operations/workflows are plain arrays.
    if ($null -eq $raw) { return @() }
    if ($raw.PSObject.Properties.Name -contains "items") { return @($raw.items) }
    return @($raw)
}

function Find-By([array]$Items, [string]$Field, [string]$Value) {
    return @($Items) | Where-Object { $_.$Field -eq $Value } | Select-Object -First 1
}

# Owner / Agent: tai khoan co dinh
function Ensure-Owner() {
    $candidate = @{ name = "Minh Anh Nguyen"; email = "owner@nexa.local"; password = $Password; industry = "ecommerce" }
    try {
        $null = Call-Api POST "/auth/register" $candidate
        Log "Da tao owner: owner@nexa.local (co starter nganh e-commerce)"
    } catch {
        if ($_.Exception.Message -match "409") {
            Note "owner@nexa.local da ton tai - dung tai khoan hien co"
        } else { throw }
    }
}
function Ensure-Agent() {
    $ownerLogin = Call-Api POST "/auth/login" @{ email = "owner@nexa.local"; password = $Password }
    if ($Mode -eq "direct") { $headers.Authorization = "Bearer $($ownerLogin.access_token)" }
    $members = Get-List "/team"
    if (Find-By $members "email" "agent@nexa.local") {
        Note "agent@nexa.local da co trong team"
    } else {
        $null = Call-Api POST "/team" @{ name = "Trai An Duong"; email = "agent@nexa.local"; password = $Password }
        Log "Da tao agent: agent@nexa.local"
    }
}

Write-Host ""
Write-Host "=== NEXA DEMO SEED (E-commerce, 1 bang) ===" -ForegroundColor Cyan
Write-Host "Che do : $Mode"
Write-Host "API    : $api"
Write-Host ""

# ---------- 1. Tai khoan ----------
Write-Host "[1/8] Tai khoan demo co dinh" -ForegroundColor Yellow
Ensure-Owner
$ownerLogin = Call-Api POST "/auth/login" @{ email = "owner@nexa.local"; password = $Password }
if ($Mode -eq "direct") { $headers.Authorization = "Bearer $($ownerLogin.access_token)" }
Ensure-Agent

# Thu nghiem login agent (de kiem tra mat khau chung lam viec duoc)
$agentLogin = Call-Api POST "/auth/login" @{ email = "agent@nexa.local"; password = $Password }
Log "agent login OK - phien agent su dung duoc mat khau chung"
if ($Mode -eq "direct") {
    $headers.Authorization = "Bearer $($ownerLogin.access_token)"
} else {
    # BFF: quay lai phien owner (cookie) de cac muc tiep theo duyen quyen owner.
    $null = Call-Api POST "/auth/login" @{ email = "owner@nexa.local"; password = $Password }
}

# ---------- 2. Khach hang ----------
Write-Host "[2/8] Ho so khach hang (8 ho so)" -ForegroundColor Yellow
$customers = @(
    @{ name = "Nguyen Thu Ha";  email = "ha.nguyen@example.vn";  phone = "0912000001" },
    @{ name = "Le Van Tuan";    email = "tuan.le@example.vn";    phone = "0912000002" },
    @{ name = "Pham Thi Lan";   email = "lan.pham@example.vn";   phone = "0912000003" },
    @{ name = "Tran Quang Dat"; email = "dat.tran@example.vn";   phone = "0912000004" },
    @{ name = "Hoang Thi My";   email = "my.hoang@example.vn";   phone = "0912000005" },
    @{ name = "Bui Minh Khoa";  email = "khoa.bui@example.vn";   phone = "0912000006" },
    @{ name = "Vu Thi Trang";   email = "trang.vu@example.vn";   phone = "0912000007" },
    @{ name = "Pham Nhat Minh"; email = "minh.pham@example.vn";  phone = "0912000008" }
)
foreach ($c in $customers) {
    if (Find-By (Get-List "/customers") "email" $c.email) {
        Note "customer $($c.email) da co"
        continue
    }
    $null = Call-Api POST "/customers" $c
    Log "Them customer: $($c.name) (<$($c.email)>)"
}

# ---------- 3. Kho kien thuc ----------
Write-Host "[3/8] Kho kien thuc (5 bai, public - de RAG + trung tam tro giup)" -ForegroundColor Yellow
$articles = @(
    @{
        subject = "Chinh sach hoan tien trong 7 ngay"
        content = "Doanh nghiep tiep nhan yeu cau hoan tien trong vong 7 ngay keu tu ngay nhan duoc hang. Khach hang can cung cap ma don va ly do. Tien duoc chuyen ve tai khoan goc trong 3-5 ngay lam vieu sau khi duyet."
    },
    @{
        subject = "Bang gia van chuyen theo khu vuc"
        content = "Khu vuc mien Bac: 25.000-35.000 dong. Mien Trung: 30.000-45.000 dong. Mien Nam: 35.000-50.000 dong. Thu do mien mien: +10.000 dong. Gia bao gom thue. Thu tra theo trong luong don."
    },
    @{
        subject = "Quy trinh COD va xu ly that bai"
        content = "Khi shipper that bai 2 lan, don tu dong chuyen sang chieu doan khach chon lai thoi gian. Neu qua 24 giu ma chua co phan hoi, he thong tao phiu ho tro va bao cao quyen tri. Khach co the doi thanh toan sang chuyen khoan."
    },
    @{
        subject = "Doi hang va trang dung"
        content = "Doi hang: nhap trong 15 ngay, don khac ma khong bi su dung. Thu tra van chuyen do khach chi. Trang dung: duyet trong 24 giu, tien chuyen ve trong 3 ngay lam vieu."
    },
    @{
        subject = "Chinh sach khuyen mai theo don"
        content = "Gia giam tu dong theo ma don khuyen mai. Khuyen mai khong ap dung dong thoi voi nhau. Neu khach dung ma sai, don co the bi treo 30 phut roi tu dong xoa."
    }
)
foreach ($a in $articles) {
    if (Find-By (Get-List "/knowledge") "subject" $a.subject) {
        Note "KB '$($a.subject)' da co"
    } else {
        $kb = @{ subject = $a.subject; content = $a.content; enabled = $true; public = $true }
        $null = Call-Api POST "/knowledge" $kb
        Log "Them KB: $($a.subject)"
    }
}

# ---------- 4. Automation ----------
Write-Host "[4/8] Tong hop tu dong (3 automation tu khoa)" -ForegroundColor Yellow
$automations = @(
    @{ name = "Tu dong tra loi tu khoa 'hoan tien'";  keywords = @("hoan tien", "trau tien") },
    @{ name = "Tu dong tra loi tu khoa 'chua nhan don'"; keywords = @("chua nhan don", "don cam") },
    @{ name = "Tu dong tra loi tu khoa 'khuyen mai'"; keywords = @("khuyen mai", "ma giam") }
)
foreach ($a in $automations) {
    if (Find-By (Get-List "/automations") "name" $a.name) {
        Note "automation '$($a.name)' da co"
    } else {
        $body = @{ name = $a.name; keywords = $a.keywords; enabled = $true }
        $null = Call-Api POST "/automations" $body
        Log "Them automation: $($a.name)"
    }
}

# ---------- 5. Hoi thoai + phieu ho tro ----------
Write-Host "[5/8] Hoi thoai don hang + phieu ho tro (demo wow)" -ForegroundColor Yellow

$customerList = Get-List "/customers"
$scenarios = @(
    @{
        email = "ha.nguyen@example.vn"; channel = "web";
        subject = "Don #20260012 chua nhan duoc"
        messages = @(
            @{ sender = "customer"; content = "Chao, toi mua don #20260012 ngay 10/06, gio bi thong bao giao thanh cong nhung toi chua nhan duoc hang. Xin kiem tra do." },
            @{ sender = "agent";    content = "Xin loi ban, chung toi da kiem tra: don #20260012 da duoc shipper hoi doan 10/06. chung toi da gap shipper xac nhan tra hang ve kho. Xin ban chao thong bao 5-7 ngay de nhan lai." }
        )
    },
    @{
        email = "tuan.le@example.vn"; channel = "email";
        subject = "Yeu cau hoan tien don #20260015"
        messages = @(
            @{ sender = "customer"; content = "Toi mua san khieu A ma don, nhung nhan don bi an duong. Toi mun hoan tien. Don #20260015." },
            @{ sender = "agent";    content = "Chung toi da tiep nhan yeu cau hoan tien don #20260015. Theo chinh sach, don duoc duyet trong 3-5 ngay lam vieu. Ban se nhan duoc email xac nhan." }
        )
    },
    @{
        email = "lan.pham@example.vn"; channel = "web";
        subject = "Ship COD that bai 2 lan - don #20260018"
        messages = @(
            @{ sender = "customer"; content = "Don #20260018 ship 2 lan deu that bai vi toi di lam. Gio toi muon chon lai thoi gian nhat hang. Ban giup toi co duoc khong?" },
            @{ sender = "agent";    content = "Duoc nha! Don #20260018 da duoc chuyen sang 'doi thoi gian nhan hang'. Ban nhat hang truoc 17h ngay hom nay, toi se lien he shipper de giao trong 1 gio." }
        )
    },
    @{
        email = "dat.tran@example.vn"; channel = "web";
        subject = "Co ship ra Can Tho khong?"
        messages = @(
            @{ sender = "customer"; content = "Toi o Can Tho, co ship khong? Bao nhieu dong va miet bao lam du?" }
        )
    },
    @{
        email = "my.hoang@example.vn"; channel = "email";
        subject = "Doi size M sang L don #20260020"
        messages = @(
            @{ sender = "customer"; content = "Don #20260020 mua ao da size M, muon doi L. Hoi lai chi tiet ve thu gia van chuyen." },
            @{ sender = "agent";    content = "Don #20260020 duoc doi hang trong vong 15 ngay. Khach chi thanh thu gia van chuyen 15.000 dong. Don doi duoc tao voi ma #20260020-R1." }
        )
    }
)

foreach ($s in $scenarios) {
    $cust = Find-By $customerList "email" $s.email
    if (-not $cust) { Note "customer $($s.email) chua co - bo qua scenario"; continue }
    $convPayload = @{ customer_id = $cust.id; channel = $s.channel; status = "open"; subject = $s.subject }
    $existingConv = $null
    # Hoi thoai duoc danh theo subject
    $convs = Get-List "/conversations"
    $existingConv = Find-By $convs "subject" $s.subject
    if ($existingConv -and $existingConv.customer_id -eq $cust.id) {
        Note "conversation '$($s.subject)' da co"
        $conv = $existingConv
    } else {
        $conv = Call-Api POST "/conversations" $convPayload
        Log "Them conversation: $($s.subject) (kh: $($s.channel))"
    }
    $msgs = Get-List "/conversations/$($conv.id)/messages"
    if (($msgs | Measure-Object).Count -eq 0) {
        foreach ($m in $s.messages) {
            try { $null = Call-Api POST "/conversations/$($conv.id)/messages" $m } catch {
                Note "Gui message loi: $($_.Exception.Message)"
            }
        }
        Log "Them $($s.messages.Count) message vao '$($s.subject)'"
    } else {
        Note "conversation da co $($msgs.Count) message - bo qua"
    }
}

# Phieu ho tro
$ticketScenarios = @(
    @{ email = "ha.nguyen@example.vn"; subject = "Nhan hang bi an duong - don #20260012"; priority = "high";   status = "open" },
    @{ email = "lan.pham@example.vn";  subject = "Yeu cau chuyen COD thanh toan sang chuyen khoan - don #20260018"; priority = "normal"; status = "in_progress" }
)
foreach ($t in $ticketScenarios) {
    $cust = Find-By $customerList "email" $t.email
    if (-not $cust) { continue }
    if (Find-By (Get-List "/tickets") "subject" $t.subject) {
        Note "ticket '$($t.subject)' da co"
    } else {
        $body = @{ subject = $t.subject; status = $t.status; priority = $t.priority; customer_id = $cust.id }
        $null = Call-Api POST "/tickets" $body
        Log "Them phieu ho tro: $($t.subject) (priority=$($t.priority))"
    }
}

# ---------- 6. Thiet lap tenant: workflow + AI Sparring ----------
Write-Host "[6/8] Thiet lap tenant (bat workflow + ai_sparring)" -ForegroundColor Yellow
function Ensure-Flags([hashtable]$Flags) {
    $settings = $null
    try { $settings = Call-Api GET "/admin/settings" $null } catch {
        Warn "Khong doc duoc /admin/settings: $($_.Exception.Message)"
        return
    }
    $flags = @{}
    if ($settings -and $settings.PSObject.Properties.Name -contains "flags" -and $settings.flags) {
        foreach ($k in $settings.flags.PSObject.Properties.Name) { $flags[$k] = [bool]$settings.flags.$k }
    }
    foreach ($k in $Flags.Keys) { $flags[$k] = [bool]$Flags[$k] }
    $body = @{ allowlist = @(); flags = $flags }
    if ($settings) {
        foreach ($prop in @("display_name", "logo_url", "accent", "host", "realtime_qa_policy")) {
            if ($settings.PSObject.Properties.Name -contains $prop -and $settings.$prop) { $body[$prop] = $settings.$prop }
        }
        if ($settings.PSObject.Properties.Name -contains "daily_ai_requests" -and $null -ne $settings.daily_ai_requests) { $body["daily_ai_requests"] = $settings.daily_ai_requests }
        if ($settings.PSObject.Properties.Name -contains "persona_sop" -and $settings.persona_sop) { $body["persona_sop"] = $settings.persona_sop }
    }
    $null = Call-Api PUT "/admin/settings" $body
    Log "Flags da bat: $($Flags.Keys -join ', ')"
}
Ensure-Flags @{ workflows = $true; ai_sparring = $true }

# ---------- 7. Workflow tu dong ----------
Write-Host "[7/8] Workflow tu dong (2 quy tac)" -ForegroundColor Yellow
function Ensure-Workflow([string]$Name, $Rule) {
    $existing = Get-List "/operations/workflows"
    if (Find-By $existing "name" $Name) {
        Note "workflow '$Name' da co"
        return
    }
    $null = Call-Api POST "/operations/workflows" $Rule
    Log "Them workflow: $Name"
}
Ensure-Workflow "Tu tao phieu khi khach bao hoan tien" @{
    name      = "Tu tao phieu khi khach bao hoan tien"
    enabled   = $true
    trigger   = "message_received"
    conditions = @{ keywords_any = @("hoan tien", "tra lai tien"); status = "open" }
    actions   = @(@{ type = "create_ticket"; value = "Yeu cau hoan tien (tu dong)" })
}
Ensure-Workflow "Nhan phieu ho tro ngung 3 ngay" @{
    name      = "Nhan phieu ho tro ngung 3 ngay"
    enabled   = $true
    trigger   = "ticket_idle"
    idle_days = 3
    conditions = @{}
    actions   = @(@{ type = "notify_owner"; value = "Phieu da im 3 ngay - can phan hoi" })
}

# ---------- 8. Kich ban AI Sparring ----------
Write-Host "[8/8] Kich ban AI Sparring (2 kich ban tu hoi thoai demo)" -ForegroundColor Yellow
function Ensure-Scenario([string]$Title, [string]$ConvSubject, [string]$Expected, [array]$Lines) {
    $convs = Get-List "/conversations"
    $conv = Find-By $convs "subject" $ConvSubject
    if (-not $conv) {
        Note "conversation '$ConvSubject' chua co - bo qua kich ban '$Title'"
        return
    }
    $body = @{
        conversation_id = $conv.id
        title           = $Title
        expected_action = $Expected
        lines           = $Lines
        anonymized      = $true
    }
    try {
        $scen = Call-Api POST "/ai/sparring/scenarios" $body
        Log "Kich ban sparring '$($scen.id)' - '$Title' (dinh: $Expected)"
    } catch {
        Warn "Tao kich ban '$Title' that bai: $($_.Exception.Message)"
    }
}
# Kich ban "giai quyet": nhan vien thuc hien trong hoi thoai -> dinh resolve_ticket
Ensure-Scenario "Ho tro doi size san khau trong 15 ngay" "Doi size M sang L don #20260020" "resolve_ticket" @(
    @{ role = "customer"; text = "Doi size M sang L don #20260020 mua ao da size M, muon doi L. Hoi lai chi tiet ve thu gia van chuyen." },
    @{ role = "agent";    text = "Don #20260020 duoc doi hang trong vong 15 ngay. Khach chi thanh thu gia van chuyen 15.000 dong. Don doi da duoc tao voi ma #20260020-R1." }
)
# Kich ban "chuyen nguoi that": can shipper / xu ly ngoai hoi thoai -> dinh handoff
Ensure-Scenario "Ship COD that bai 2 lan - chon lai thoi gian nhan hang" "Ship COD that bai 2 lan - don #20260018" "handoff" @(
    @{ role = "customer"; text = "Don #20260018 ship 2 lan deu that bai vi toi di lam. Gio toi muon chon lai thoi gian nhat hang. Ban giup toi co duoc khong?" },
    @{ role = "agent";    text = "Duoc nha! Don #20260018 da duoc chuyen sang 'doi thoi gian nhan hang'. Ban nhat hang truoc 17h ngay hom nay, toi se lien he shipper de giao trong 1 gio." }
)

# ---------- Xac nhan dashboard ----------
Write-Host ""
$dash = $null
try { $dash = Call-Api GET "/dashboard" $null } catch { }
if ($dash) {
    Write-Host "Dashboard hien tai: $(($dash | ConvertTo-Json -Compress -Depth 5).Substring(0, [Math]::Min(200, ($dash | ConvertTo-Json -Compress -Depth 5).Length)))" -ForegroundColor DarkGray
}

# ---------- Toa ----------
Write-Host ""
Write-Host "=== SEED HOAN THANH ===" -ForegroundColor Green
Write-Host ""
Write-Host "Tai khoan demo (fixed - dung de login UI):"
Write-Host "  Owner : owner@nexa.local / $Password"
Write-Host "  Agent : agent@nexa.local / $Password"
Write-Host ""
Write-Host "Du lieu da seed (1 bang, nganh e-commerce):"
Write-Host "  - 8 ho so khach hang"
Write-Host "  - 5 bai KB public (hoan tien, van chuyen, COD, doi hang, khuyen mai)"
Write-Host "  - 3 automation tu khoa (hoan tien / chua nhan don / khuyen mai)"
Write-Host "  - 5 hoi thoai don hang: chua nhan don, hoan tien, COD that bai, van chuyen Can Tho, doi size"
Write-Host "  - 2 phieu ho tro: don hang bi an duong (high), chuyen COD sang chuyen khoan (normal)"
Write-Host "  - 2 workflow tu dong: tin nhap chua tu tao phieu + nhan phieu ngung 3 ngay"
Write-Host "  - 2 kich ban AI Sparring: doi size (resolve) + COD that bai (handoff)"
Write-Host ""
Write-Host "Ghi chu: owner co du lieu demo, agent chi nhan don. Chay lai script khong tao trung."
Write-Host "AI Sparring can OPENAI_API_KEY (hoac LLM_PROVIDER) de bat dau phien luyen t?."
Write-Host "Cham diem / Cham kiem dinh AI agent chay nen sau 10-20 giay (hang doi AI jobs)."
Write-Host ""
