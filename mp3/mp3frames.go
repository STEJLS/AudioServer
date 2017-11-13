package mp3

import (
	"fmt"
	"time"
)

type (
	version     byte
	layer       byte
	channelMode byte
	emphasis    byte
	frameHeader struct {
		version         version
		layer           layer
		Protection      bool
		Bitrate         int
		SampleRate      int
		Pad             bool
		Private         bool
		channelMode     channelMode
		IntensityStereo bool
		MSStereo        bool
		CopyRight       bool
		Original        bool
		emphasis        emphasis

		Size     int64
		Samples  int
		Duration time.Duration
	}
)

func init() {
	//Табличные данные битрейта и сэмплов на фрейм
	//для версий  mPEG2.5 и mPEG2 совпадают.
	bitrates[mPEG25] = bitrates[mPEG2]
	samplesPerFrame[mPEG25] = samplesPerFrame[mPEG2]
}

func (this *frameHeader) Parse(data []byte) error {
	this.Size = 0
	this.Samples = 0
	this.Duration = 0

	if len(data) < 4 {
		return fmt.Errorf("Слишком мало байт для заголовка!")
	}
	if data[0] != 0xFF || (data[1]&0xE0) != 0xE0 {
		return fmt.Errorf("Нет битов синхронизации(первые 11 бит единиц), получено: %x, %x", data[0], data[1])
	}
	this.version = version((data[1] >> 3) & 0x03)
	this.layer = layer(((data[1] >> 1) & 0x03))
	this.Protection = (data[1] & 0x01) != 0x01

	bitrateIdx := (data[2] >> 4) & 0x0F
	if bitrateIdx == 0x0F {
		return fmt.Errorf("Неверный бит рейт: %v\n", bitrateIdx)
	}
	this.Bitrate = bitrates[this.version][this.layer][bitrateIdx] * 1000
	if this.Bitrate == 0 {
		return fmt.Errorf("Неверный бит рейт: %v\n", bitrateIdx)
	}

	sampleRateIdx := (data[2] >> 2) & 0x03
	if sampleRateIdx == 0x03 {
		return fmt.Errorf("Неверный сэмпл рейт: %v", sampleRateIdx)
	}
	this.SampleRate = sampleRates[this.version][sampleRateIdx]
	this.Pad = ((data[2] >> 1) & 0x01) == 0x01
	this.Private = (data[2] & 0x01) == 0x01
	this.channelMode = channelMode(data[3]>>6) & 0x03
	this.CopyRight = (data[3]>>3)&0x01 == 0x01
	this.Original = (data[3]>>2)&0x01 == 0x01
	this.emphasis = emphasis(data[3] & 0x03)

	this.Size = this.size()
	this.Samples = this.samples()
	this.Duration = this.duration()

	return nil
}

func (this *frameHeader) samples() int {
	return samplesPerFrame[this.version][this.layer]
}

func (this *frameHeader) size() int64 {
	bps := float64(this.samples()) / 8.0
	fsize := (bps * float64(this.Bitrate)) / float64(this.SampleRate)
	if this.Pad {
		fsize += float64(slotSize[this.layer])
	}
	return int64(fsize)
}

func (this *frameHeader) duration() time.Duration {
	ms := (1000 / float64(this.SampleRate)) * float64(this.samples())
	return time.Duration(time.Duration(float64(time.Millisecond) * ms))
}
