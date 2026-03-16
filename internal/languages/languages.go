package languages

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

const (
	autoDetectCode = "auto"
	unknownLabel   = "Unknown"
)

// Option is a selectable language exposed by the UI.
type Option struct {
	Code    string            `json:"code"`
	Label   string            `json:"label"`
	Labels  map[string]string `json:"labels"`
	Visible bool              `json:"visible"`
}

var supportedUILocales = []struct {
	Code string
	Tag  language.Tag
}{
	{Code: "en", Tag: language.English},
	{Code: "zh-CN", Tag: language.SimplifiedChinese},
	{Code: "ja", Tag: language.Japanese},
	{Code: "ko", Tag: language.Korean},
	{Code: "de-DE", Tag: language.German},
	{Code: "fr", Tag: language.French},
	{Code: "es", Tag: language.Spanish},
}

var supportedUILocaleMatcher = language.NewMatcher([]language.Tag{
	language.English,
	language.SimplifiedChinese,
	language.Japanese,
	language.Korean,
	language.German,
	language.French,
	language.Spanish,
})

var autoDetectLabels = map[string]string{
	"en":    "Auto Detect",
	"zh-CN": "\u81ea\u52a8\u68c0\u6d4b",
	"ja":    "\u81ea\u52d5\u691c\u51fa",
	"ko":    "\uc790\ub3d9 \uac10\uc9c0",
	"de-DE": "Automatisch erkennen",
	"fr":    "D\u00e9tection automatique",
	"es":    "Detecci\u00f3n autom\u00e1tica",
}

var commonVisibleLanguageCodes = []string{
	"en",
	"zh-CN",
	"zh-TW",
	"ja",
	"ko",
	"de",
	"fr",
	"es",
	"ru",
	"ar",
}

var displayTagOverrides = map[string]language.Tag{
	"zh-CN": language.SimplifiedChinese,
	"zh-TW": language.TraditionalChinese,
}

var supported = buildSupportedOptions()

// Supported returns all UI language options.
func Supported() []Option {
	return cloneOptions(supported)
}

// WithoutAuto returns all language options except auto-detect.
func WithoutAuto() []Option {
	out := make([]Option, 0, len(supported)-1)
	for _, item := range supported {
		if item.Code == autoDetectCode || !item.Visible {
			continue
		}
		out = append(out, cloneOption(item))
	}
	return out
}

// Label returns the English display label for a language code.
func Label(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return unknownLabel
	}
	return LanguageCodeToLanguageName(language.English, code)
}

// LanguageCodeToLanguageName returns a localized language name for the given UI locale.
func LanguageCodeToLanguageName(uiLocale language.Tag, languageCode string) string {
	code := strings.TrimSpace(languageCode)
	if code == "" {
		return unknownLabel
	}
	if code == autoDetectCode {
		return localizedAutoDetectLabel(uiLocale)
	}

	tag, err := languageCodeTag(code)
	if err != nil {
		return code
	}
	if name := languageDisplayName(uiLocale, tag, code); name != "" {
		return name
	}
	if name := languageDisplayName(language.English, tag, code); name != "" {
		return name
	}
	return code
}

func localizedAutoDetectLabel(uiLocale language.Tag) string {
	_, index, _ := supportedUILocaleMatcher.Match(uiLocale)
	if index >= 0 && index < len(supportedUILocales) {
		return autoDetectLabels[supportedUILocales[index].Code]
	}
	return autoDetectLabels["en"]
}

func languageCodeTag(code string) (language.Tag, error) {
	if tag, ok := displayTagOverrides[code]; ok {
		return tag, nil
	}
	return language.Parse(code)
}

func languageDisplayName(uiLocale, tag language.Tag, rawCode string) string {
	name := strings.TrimSpace(display.Languages(uiLocale).Name(tag))
	if name == "" {
		return ""
	}
	if strings.EqualFold(name, rawCode) || strings.EqualFold(name, tag.String()) {
		return ""
	}
	return name
}

func buildSupportedOptions() []Option {
	codes := strings.Fields(rawSupportedLanguageCodes)
	out := make([]Option, 0, len(codes)+1)
	seenLabels := map[string]struct{}{}
	auto := buildOption(autoDetectCode)
	auto.Visible = true
	out = append(out, auto)
	seenLabels[visibilityKey(auto.Label)] = struct{}{}
	for _, code := range codes {
		option := buildOption(code)
		key := visibilityKey(option.Label)
		_, hiddenVariant := seenLabels[key]
		option.Visible = !hiddenVariant
		if !hiddenVariant {
			seenLabels[key] = struct{}{}
		}
		out = append(out, option)
	}
	sortSupportedOptions(out)
	return out
}

func buildOption(code string) Option {
	labels := make(map[string]string, len(supportedUILocales))
	for _, locale := range supportedUILocales {
		labels[locale.Code] = LanguageCodeToLanguageName(locale.Tag, code)
	}
	return Option{
		Code:    code,
		Label:   LanguageCodeToLanguageName(language.English, code),
		Labels:  labels,
		Visible: false,
	}
}

func visibilityKey(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

func sortSupportedOptions(options []Option) {
	commonRanks := make(map[string]int, len(commonVisibleLanguageCodes))
	for index, code := range commonVisibleLanguageCodes {
		commonRanks[code] = index
	}

	sort.SliceStable(options, func(i, j int) bool {
		left := options[i]
		right := options[j]

		if left.Code == autoDetectCode || right.Code == autoDetectCode {
			return left.Code == autoDetectCode
		}

		if left.Visible != right.Visible {
			return left.Visible
		}

		leftRank, leftCommon := commonRanks[left.Code]
		rightRank, rightCommon := commonRanks[right.Code]
		if left.Visible && leftCommon != rightCommon {
			return leftCommon
		}
		if left.Visible && leftCommon && rightCommon && leftRank != rightRank {
			return leftRank < rightRank
		}

		if byLabel := cmp.Compare(left.Label, right.Label); byLabel != 0 {
			return byLabel < 0
		}
		return slices.Compare([]string{left.Code}, []string{right.Code}) < 0
	})
}

func cloneOptions(options []Option) []Option {
	out := make([]Option, len(options))
	for i, option := range options {
		out[i] = cloneOption(option)
	}
	return out
}

func cloneOption(option Option) Option {
	labels := make(map[string]string, len(option.Labels))
	for key, value := range option.Labels {
		labels[key] = value
	}
	return Option{
		Code:    option.Code,
		Label:   option.Label,
		Labels:  labels,
		Visible: option.Visible,
	}
}

// Keep this list in sync with the language codes embedded in the TranslateGemma runtime
// chat template. `zh-CN` is included as a compatibility alias because existing app state
// and defaults already use it.
const rawSupportedLanguageCodes = `
aa
aa-DJ
aa-ER
ab
af
af-NA
ak
am
an
ar
ar-AE
ar-BH
ar-DJ
ar-DZ
ar-EG
ar-EH
ar-ER
ar-IL
ar-IQ
ar-JO
ar-KM
ar-KW
ar-LB
ar-LY
ar-MA
ar-MR
ar-OM
ar-PS
ar-QA
ar-SA
ar-SD
ar-SO
ar-SS
ar-SY
ar-TD
ar-TN
ar-YE
as
az
az-Arab
az-Arab-IQ
az-Arab-TR
az-Cyrl
az-Latn
ba
be
be-tarask
bg
bg-BG
bm
bm-Nkoo
bn
bn-IN
bo
bo-IN
br
bs
bs-Cyrl
bs-Latn
ca
ca-AD
ca-ES
ca-FR
ca-IT
ce
co
cs
cs-CZ
cv
cy
da
da-DK
da-GL
de
de-AT
de-BE
de-CH
de-DE
de-IT
de-LI
de-LU
dv
dz
ee
ee-TG
el
el-CY
el-GR
el-polyton
en
en-AE
en-AG
en-AI
en-AS
en-AT
en-AU
en-BB
en-BE
en-BI
en-BM
en-BS
en-BW
en-BZ
en-CA
en-CC
en-CH
en-CK
en-CM
en-CX
en-CY
en-CZ
en-DE
en-DG
en-DK
en-DM
en-ER
en-ES
en-FI
en-FJ
en-FK
en-FM
en-FR
en-GB
en-GD
en-GG
en-GH
en-GI
en-GM
en-GS
en-GU
en-GY
en-HK
en-HU
en-ID
en-IE
en-IL
en-IM
en-IN
en-IO
en-IT
en-JE
en-JM
en-KE
en-KI
en-KN
en-KY
en-LC
en-LR
en-LS
en-MG
en-MH
en-MO
en-MP
en-MS
en-MT
en-MU
en-MV
en-MW
en-MY
en-NA
en-NF
en-NG
en-NL
en-NO
en-NR
en-NU
en-NZ
en-PG
en-PH
en-PK
en-PL
en-PN
en-PR
en-PT
en-PW
en-RO
en-RW
en-SB
en-SC
en-SD
en-SE
en-SG
en-SH
en-SI
en-SK
en-SL
en-SS
en-SX
en-SZ
en-TC
en-TK
en-TO
en-TT
en-TV
en-TZ
en-UG
en-UM
en-VC
en-VG
en-VI
en-VU
en-WS
en-ZA
en-ZM
en-ZW
eo
es
es-AR
es-BO
es-BR
es-BZ
es-CL
es-CO
es-CR
es-CU
es-DO
es-EA
es-EC
es-ES
es-GQ
es-GT
es-HN
es-IC
es-MX
es-NI
es-PA
es-PE
es-PH
es-PR
es-PY
es-SV
es-US
es-UY
es-VE
et
et-EE
eu
fa
fa-AF
fa-IR
ff
ff-Adlm
ff-Adlm-BF
ff-Adlm-CM
ff-Adlm-GH
ff-Adlm-GM
ff-Adlm-GW
ff-Adlm-LR
ff-Adlm-MR
ff-Adlm-NE
ff-Adlm-NG
ff-Adlm-SL
ff-Adlm-SN
ff-Latn
ff-Latn-BF
ff-Latn-CM
ff-Latn-GH
ff-Latn-GM
ff-Latn-GN
ff-Latn-GW
ff-Latn-LR
ff-Latn-MR
ff-Latn-NE
ff-Latn-NG
ff-Latn-SL
fi
fi-FI
fil-PH
fo
fo-DK
fr
fr-BE
fr-BF
fr-BI
fr-BJ
fr-BL
fr-CA
fr-CD
fr-CF
fr-CG
fr-CH
fr-CI
fr-CM
fr-DJ
fr-DZ
fr-FR
fr-GA
fr-GF
fr-GN
fr-GP
fr-GQ
fr-HT
fr-KM
fr-LU
fr-MA
fr-MC
fr-MF
fr-MG
fr-ML
fr-MQ
fr-MR
fr-MU
fr-NC
fr-NE
fr-PF
fr-PM
fr-RE
fr-RW
fr-SC
fr-SN
fr-SY
fr-TD
fr-TG
fr-TN
fr-VU
fr-WF
fr-YT
fy
ga
ga-GB
gd
gl
gn
gu
gu-IN
gv
ha
ha-Arab
ha-Arab-SD
ha-GH
ha-NE
he
he-IL
hi
hi-IN
hi-Latn
hr
hr-BA
hr-HR
ht
hu
hu-HU
hy
ia
id
id-ID
ie
ig
ii
ik
io
is
it
it-CH
it-IT
it-SM
it-VA
iu
iu-Latn
ja
ja-JP
jv
ka
ki
kk
kk-Arab
kk-Cyrl
kk-KZ
kl
km
kn
kn-IN
ko
ko-CN
ko-KP
ko-KR
ks
ks-Arab
ks-Deva
ku
kw
ky
la
lb
lg
ln
ln-AO
ln-CF
ln-CG
lo
lt
lt-LT
lu
lv
lv-LV
mg
mi
mk
ml
ml-IN
mn
mn-Mong
mn-Mong-MN
mr
mr-IN
ms
ms-Arab
ms-Arab-BN
ms-BN
ms-ID
ms-SG
mt
my
nb
nb-SJ
nd
ne
ne-IN
nl
nl-AW
nl-BE
nl-BQ
nl-CW
nl-NL
nl-SR
nl-SX
nn
no
no-NO
nr
nv
ny
oc
oc-ES
om
om-KE
or
os
os-RU
pa
pa-IN
pa-Arab
pa-Guru
pl
pl-PL
ps
ps-PK
pt
pt-AO
pt-BR
pt-CH
pt-CV
pt-GQ
pt-GW
pt-LU
pt-MO
pt-MZ
pt-PT
pt-ST
pt-TL
qu
qu-BO
qu-EC
rm
rn
ro
ro-MD
ro-RO
ru
ru-BY
ru-KG
ru-KZ
ru-MD
ru-RU
ru-UA
rw
sa
sc
sd
sd-Arab
sd-Deva
se
se-FI
se-SE
sg
si
sk
sk-SK
sl
sl-SI
sn
so
so-DJ
so-ET
so-KE
sq
sq-MK
sq-XK
sr
sr-RS
sr-Cyrl
sr-Cyrl-BA
sr-Cyrl-ME
sr-Cyrl-XK
sr-Latn
sr-Latn-BA
sr-Latn-ME
sr-Latn-XK
ss
ss-SZ
st
st-LS
su
su-Latn
sv
sv-AX
sv-FI
sv-SE
sw
sw-CD
sw-KE
sw-TZ
sw-UG
ta
ta-IN
ta-LK
ta-MY
ta-SG
te
te-IN
tg
th
th-TH
ti
ti-ER
tk
tl
tn
tn-BW
to
tr
tr-CY
tr-TR
ts
tt
ug
uk
uk-UA
ur
ur-IN
ur-PK
uz
uz-Arab
uz-Cyrl
uz-Latn
ve
vi
vi-VN
vo
wa
wo
xh
yi
yo
yo-BJ
za
zh
zh-CH
zh-CN
zh-TW
zh-Hans
zh-Hans-HK
zh-Hans-MO
zh-Hans-MY
zh-Hans-SG
zh-Hant
zh-Hant-HK
zh-Hant-MO
zh-Hant-MY
zh-Latn
zu
zu-ZA
`
