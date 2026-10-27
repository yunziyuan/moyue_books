package services

// Edge TTS Sec-MS-GEC v2 令牌生成（对齐官方 Python edge_tts 库 drm.py 的
// generate_sec_ms_gec_v2）。微软已轮换 GEC 校验：旧版 v1（单纯 SHA256）令牌与
// 旧 GEC 版本前缀 "1-xxx" 均被拒，必须使用 v2（CS64-SHA256 两段式）令牌与
// "2-xxx" 版本前缀，否则握手被服务器拒绝（表现为 websocket: bad handshake）。
//
// 实现严格移植自官方 edge_tts：
//   - 改版 SHA256（首 14 字调度带定制初始状态/常量）
//   - CS64 摘要
// 两者组合成 "<16hex>-<64hex>" 形式的 v2 令牌。

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

// ── 标准 SHA256 轮常量（64 个）──────────────────────────────────────
var gecSHA256K = [64]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

// gecBlockConstants 标准轮常量，但索引 14 处替换为定制调整量（0xC9DC06A7）。
// 仅在 v2 令牌的"定制初始状态"预计算阶段（firstRound==14）使用。
var gecBlockConstants = func() [64]uint32 {
	k := gecSHA256K
	k[14] = 0xC9DC06A7
	return k
}()

// ── 改版 SHA256 的定制常量 ──────────────────────────────────────────
var gecSHA256Initial = [8]uint32{
	0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
	0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
}
var gecSHA256State = [8]uint32{
	0xFD9FBEA2, 0x91243CE7, 0xBE499094, 0xCD2E1E0A,
	0x6C006F49, 0x04BD35AB, 0xFEB0A7E4, 0x03EFFBD2,
}
var gecSHA256Schedule = [14]uint32{
	0xEB24BF9E, 0x1558691F, 0xA0C09EE2, 0x0CFA32BA, 0xA622EE8C, 0xD1DD9491, 0xC0DD25A0,
	0xF9E70AB6, 0xF0C31573, 0x7058CE4E, 0x7188E606, 0x00E5AB33, 0xD258B060, 0x72766564,
}

const (
	gecSHA256Adjustment = 0xC9DC06A7
	gecSigma0Adjustment = 0x059C0B80
	gecSigma1Adjustment = 0x000B92C0
	gecU32Mask          = 0xFFFFFFFF
)

// ── CS64 摘要的定制常量 ────────────────────────────────────────────
var gecCS64Coefficients = [14][2]uint32{
	{0xE82E91B1, 0xB32700D0}, {0x44DE24C6, 0x88DFF51D}, {0x3BEB65BC, 0x29514038},
	{0xC89C5933, 0xB328CBDF}, {0x8D1EC211, 0xD405B4D5}, {0x314EF5D5, 0x26E55F3C},
	{0xE92491B1, 0x607801C6}, {0xF0A5B9D5, 0x82B2C1FB}, {0x06064CF3, 0xFA51A3DA},
	{0xDA79FA60, 0x207CC464}, {0xBED20560, 0x3C3C74F3}, {0x89F9C9F8, 0xD2D14B61},
	{0x35FA38C6, 0xBB04A2C1}, {0xCB49E3C6, 0xC5626167},
}

const (
	gecCS64Factor      = 0x82893933
	gecCS64Adjustment0 = 0xF2F90B1A
	gecCS64Adjustment1 = 0x0B1A0000
)

// ── 位运算辅助 ─────────────────────────────────────────────────────
func gecRotR(x uint32, n uint) uint32 { return (x >> n) | (x << (32 - n)) }

func gecSigma1(x uint32) uint32    { return gecRotR(x, 6) ^ gecRotR(x, 11) ^ gecRotR(x, 25) }
func gecSigma0(x uint32) uint32    { return gecRotR(x, 2) ^ gecRotR(x, 13) ^ gecRotR(x, 22) }
func gecCh(x, y, z uint32) uint32  { return z ^ (x & (y ^ z)) }
func gecMaj(x, y, z uint32) uint32 { return (x & y) | (z & (x | y)) }

// 消息调度用的 Sigma（与标准 SHA256 调度 Sigma 等价）
func gecSchedSigma1(x uint32) uint32 { return gecRotR(x, 17) ^ gecRotR(x, 19) ^ (x >> 10) }
func gecSchedSigma0(x uint32) uint32 { return gecRotR(x, 7) ^ gecRotR(x, 18) ^ (x >> 3) }

func gecRol16(value uint32) uint32 { return ((value << 16) | (value >> 16)) & gecU32Mask }

func gecWordsLE4Even(data []byte) []uint32 {
	padded := make([]byte, len(data))
	copy(padded, data)
	for len(padded)%4 != 0 {
		padded = append(padded, 0)
	}
	if len(padded) == 0 {
		return nil
	}
	n := len(padded) / 4
	words := make([]uint32, n)
	for i := 0; i < n; i++ {
		words[i] = binary.LittleEndian.Uint32(padded[i*4:])
	}
	if len(words)%2 != 0 {
		words = append(words, 0)
	}
	return words
}

// gecSHA256Rounds 执行 SHA256 压缩轮（firstRound 仅用于 v2 的定制初始状态预处理）。
// 这里采用标准 SHA256 轮实现（已与官方 edge_tts 的 _sha256_rounds 交叉验证逐字节一致），
// 而非其"交错"展开端口——后者虽数学等价但易移植出错，标准实现更清晰可靠。
// 内部加法均为 uint32，天然取模 2^32；firstRound 控制跳过的前若干轮（定制初始状态仅跑 14..63）。
func gecSHA256Rounds(state [8]uint32, words []uint32, firstRound int) [8]uint32 {
	a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
	var k []uint32
	if firstRound == 14 {
		k = gecBlockConstants[:]
	} else {
		k = gecSHA256K[:]
	}
	for i := firstRound; i < 64; i++ {
		t1 := h + gecSigma1(e) + gecCh(e, f, g) + k[i] + words[i]
		t2 := gecSigma0(a) + gecMaj(a, b, c)
		newA := t1 + t2
		newE := d + t1
		h, g, f, e, d, c, b, a = g, f, e, newE, c, b, a, newA
	}
	return [8]uint32{a, b, c, d, e, f, g, h}
}

// gecExpandSchedule 原地扩展消息调度到 64 字（从 firstWord 开始）。
func gecExpandSchedule(words *[]uint32, firstWord int) {
	for index := firstWord; index < 64; index++ {
		x := (*words)[index-2]
		y := (*words)[index-15]
		s1 := gecSchedSigma1(x)
		s0 := gecSchedSigma0(y)
		val := (s1 + (*words)[index-7] + s0 + (*words)[index-16]) & gecU32Mask
		*words = append(*words, val)
	}
}

// gecSHA256Init 按 v2 算法由 data 前 7 字节派生定制初始 IV。
func gecSHA256Init(data []byte) [8]uint32 {
	words := make([]uint32, 14) // 全 0
	var d3 uint32
	if len(data) >= 3 {
		d3 = binary.BigEndian.Uint32(append([]byte{0}, data[:3]...))
	}
	words = append(words, d3)
	var d47 uint32
	if len(data) >= 7 {
		d47 = binary.BigEndian.Uint32(data[3:7])
	}
	words = append(words, d47)
	for index := 16; index < 30; index++ {
		value := gecSHA256Schedule[index-16]
		if index-2 >= 14 {
			if index == 16 {
				value += gecSchedSigma1(words[index-2]) ^ gecSigma1Adjustment
			} else {
				value += gecSchedSigma1(words[index-2])
			}
		}
		if index-7 >= 14 {
			value += words[index-7]
		}
		if index-15 >= 14 {
			if index == 29 {
				value += gecSchedSigma0(words[index-15]) ^ gecSigma0Adjustment
			} else {
				value += gecSchedSigma0(words[index-15])
			}
		}
		words = append(words, value&gecU32Mask)
	}
	last := (gecSchedSigma1(words[28]) + words[23] + gecSchedSigma0(words[15]) + words[14] +
		gecSHA256Adjustment - gecSHA256K[14]) & gecU32Mask
	words = append(words, last)
	gecExpandSchedule(&words, 31)
	working := gecSHA256Rounds(gecSHA256State, words, 14)
	var out [8]uint32
	for i := 0; i < 8; i++ {
		out[i] = (gecSHA256Initial[i] + working[i]) & gecU32Mask
	}
	return out
}

// gecSHA256Digest 改版 SHA256（首块用定制 IV，后续标准）返回 32 字节。
func gecSHA256Digest(data []byte) []byte {
	length := 57 + len(data)
	padded := append([]byte{}, data[7:]...)
	padded = append(padded, 0x80)
	zeros := ((55-length)%64 + 64) % 64
	for i := 0; i < zeros; i++ {
		padded = append(padded, 0)
	}
	lenField := make([]byte, 8)
	binary.BigEndian.PutUint64(lenField, uint64(length)*8)
	padded = append(padded, lenField...)

	state := gecSHA256Init(data[:7])
	for off := 0; off < len(padded); off += 64 {
		blk := padded[off : off+64]
		words := make([]uint32, 16)
		for i := 0; i < 16; i++ {
			words[i] = binary.BigEndian.Uint32(blk[i*4:])
		}
		gecExpandSchedule(&words, 16)
		working := gecSHA256Rounds(state, words, 0)
		for i := 0; i < 8; i++ {
			state[i] = (state[i] + working[i]) & gecU32Mask
		}
	}
	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[i*4:], state[i])
	}
	return out
}

// gecCS64Digest 计算 v2 令牌的 CS64 摘要（两段 32 位小端）。
func gecCS64Digest(data []byte, digest []byte) [2]uint32 {
	keyA := binary.LittleEndian.Uint32(digest[0:4]) | 1
	keyB := binary.LittleEndian.Uint32(digest[4:8]) | 1
	factors := [2]uint32{(keyA * gecCS64Factor) & gecU32Mask, (keyB * gecCS64Factor) & gecU32Mask}
	terms := make([]uint32, 0, 28)
	offsets := make([]uint32, 0, 28)
	for i := 0; i < 14; i++ {
		f := factors[0]
		if i%2 == 1 {
			f = factors[1]
		}
		terms = append(terms, (gecCS64Coefficients[i][0]*f)&gecU32Mask)
		offsets = append(offsets, gecCS64Coefficients[i][1])
	}
	for idx, word := range gecWordsLE4Even(append([]byte{0}, data...)) {
		var term, offset uint32
		if idx%2 == 0 {
			term = word * keyA
			offset = gecRol16(word) * 0xEF0569FB
		} else {
			term = word * keyB
			offset = gecRol16(word) * 0xC31713DB
		}
		if idx == 0 {
			term += gecCS64Adjustment0 * factors[0]
			offset += gecCS64Adjustment1
		}
		terms = append(terms, term&gecU32Mask)
		offsets = append(offsets, offset&gecU32Mask)
	}
	var swapAcc, swapLast, state, acc uint32
	for i := 0; i < len(terms); i += 2 {
		x := terms[i]
		y := terms[i+1]
		t := gecRol16((offsets[i] + x) & gecU32Mask)
		q := (t * 0x76F50F34) & gecU32Mask
		swapAcc = (swapAcc + ((gecRol16(q)*0xC3EFEA97 + q) & gecU32Mask)) & gecU32Mask
		t = gecRol16((offsets[i+1] + y) & gecU32Mask)
		q = (t * 0x1881CE3C) & gecU32Mask
		swapLast = ((gecRol16(q)*0x35BD1EC9 + q) & gecU32Mask)
		swapAcc = (swapAcc + swapLast) & gecU32Mask
		state = (state*keyA + x) & gecU32Mask
		state = (gecRol16(state) * 0xCF98B111) & gecU32Mask
		state = (gecRol16(state) * 0x87085B9F) & gecU32Mask
		state = (gecRol16(state) * 0x12CEB96D) & gecU32Mask
		state = (gecRol16(state) * 0x257E1D83) & gecU32Mask
		acc = (acc + state) & gecU32Mask
		t = (state*keyB + y) & gecU32Mask
		t = (gecRol16(t) * 0xA27416F5) & gecU32Mask
		t = (gecRol16(t) * 0xD38396FF) & gecU32Mask
		state = (gecRol16((gecRol16(t)*0x7C932B89)&gecU32Mask) * 0xBFA49F69) & gecU32Mask
		acc = (acc + state) & gecU32Mask
	}
	return [2]uint32{swapLast ^ state, swapAcc ^ acc}
}

// gecTokenHash 返回 (cs64Hex, sha256Hex)，对齐官方 _token_hash。
func gecTokenHash(data []byte) (string, string) {
	digest := gecSHA256Digest(data)
	cs64 := gecCS64Digest(data, digest)
	cs64Bytes := make([]byte, 8)
	binary.LittleEndian.PutUint32(cs64Bytes[0:4], cs64[0])
	binary.LittleEndian.PutUint32(cs64Bytes[4:8], cs64[1])
	cs64Hex := strings.ToUpper(hex.EncodeToString(cs64Bytes))
	shaHex := strings.ToUpper(hex.EncodeToString(digest))
	return cs64Hex, shaHex
}

// gecGenerateSecMSGECSpecific 用给定 unix 时间戳（已含时钟偏移）生成 v2 令牌。
// 严格复刻官方浮点运算：ticks=(unix+WinEpoch) 对齐到 300s 后再 ×1e7，再用
// "%.0f" 取整，确保与官方库（及服务器校验）完全一致。
func gecGenerateSecMSGECSpecific(unix float64) string {
	ticks := unix + float64(edgeTTSWinEpoch)
	ticks -= math.Mod(ticks, 300)
	ticks *= edgeTTSSToNS / 100 // 1e9 / 100 = 1e7
	strToHash := fmt.Sprintf("%.0f%s", ticks, edgeTTSTrustedClientToken)
	cs64, sha := gecTokenHash([]byte(strToHash))
	return cs64 + "-" + sha
}
