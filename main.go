package main

import (
	"crypto/sha1"
	"net"
	// "encoding/binary"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"unicode"
)

const (
	protocolString = "BitTorrent protocol"
)

func main() {
	args := os.Args[1]
	switch args {
	case "decode":
		input := os.Args[2]
		decodeBencodedInput(input)
	case "info":
		fileName := os.Args[2]
		content, err := os.ReadFile(fileName)
		if err != nil {
			log.Fatalln(err)
		}
		decodedValue := decodeBencodedInput(string(content))
		printJson(decodedValue)
		encodedValue := bencoder(decodedValue["info"].(map[string]interface{}))
		hash := calculateHash(encodedValue)
		piecesHash := piecesHash(decodedValue["info"].(map[string]interface{})["pieces"].(string), 20)
		torrent := TorrentInfo{
			InfoHash:     hash,
			DecodedInput: decodedValue,
			PiecesHash:   piecesHash,
		}
		peers := torrent.getPeers()
		fmt.Println("Peers: ", peers)
		torrent.connectWithPeer(peers[0])
	}
}

func (t TorrentInfo) connectWithPeer(ip string) {
	conn, err := net.Dial("tcp", ip)
	if err != nil {
		log.Fatalf("Failed establishing tcp connection: %s", err)
	}
	defer conn.Close()
	handshake := t.getHandshake()
	fmt.Println("handshake: ", handshake)
	_, err = conn.Write(handshake)
	if err != nil {
		log.Fatalf("Failed handshaking: %s", err)
	}
	response := make([]byte, 68)
	_, err = conn.Read(response)
	if err != nil {
		log.Fatalf("Failed reading response: %s", err)
	}
	fmt.Printf("Peer ID: %s\n", hex.EncodeToString(response[48:]))
}
// func getPeerMessage() {
// 	message := []byte{}
// 	messageId := 5
// 	message[4:] = byte(messageId)
// }
func (t TorrentInfo) getHandshake() []byte {
	infoHashBytes, _ := hex.DecodeString(t.InfoHash)
	pstrlen := byte(19)
	pstr := []byte("BitTorrent protocol")
	reserved := make([]byte, 8)
	handshake := append([]byte{pstrlen}, pstr...)
	handshake = append(handshake, reserved...)
	handshake = append(handshake, infoHashBytes...)
	handshake = append(handshake, []byte{0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9}...)
	return handshake
}

type TorrentInfo struct {
	DecodedInput map[string]interface{}
	InfoHash     string
	PiecesHash   []string
}

func (t TorrentInfo) downloadTorrentFile() {

}
func (t TorrentInfo) getPeers() []string {
	length := t.DecodedInput["info"].(map[string]interface{})["length"].(int)
	infoHashBytes, _ := hex.DecodeString(t.InfoHash)
	announce := t.DecodedInput["announce"].(string)
	params := url.Values{}
	params.Add("peer_id", "00112233445566778899")
	params.Add("info_hash", string(infoHashBytes))
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("port", "6881")
	params.Add("left", fmt.Sprint(length))
	params.Add("compact", "1")
	url := fmt.Sprintf("%s?%s", announce, params.Encode())

	fmt.Println(url)
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Error sending request to announce: %s", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed reading body from response: %s", err)
	}
	fmt.Println(string(body))
	decodedResponse := decodeBencodedInput(string(body))
	if _, ok := decodedResponse["peers"].(string); !ok {
		log.Fatalf("Failed to get key 'peers' from response")
	}
	encodedPeers := decodedResponse["peers"].(string)
	peersInHex := piecesHash(encodedPeers, 6)
	peers := decodePeerIp(peersInHex)
	return peers
}
func decodePeerIp(hexEncodedPeers []string) []string {
	decodedIPs := []string{}
	for _, encodedPeer := range hexEncodedPeers {
		peerInHex, _ := hex.DecodeString(encodedPeer)
		bytes := []byte(peerInHex)
		ip := fmt.Sprintf("%d.%d.%d.%d:%d",
			bytes[0],
			bytes[1],
			bytes[2],
			bytes[3],
			(binary.BigEndian.Uint16)([]byte(bytes[4:])),
		)
		decodedIPs = append(decodedIPs, ip)
	}
	return decodedIPs
}

func piecesHash(piece string, byteLength int) []string {
	hashes := []string{}
	for i := 0; i < len(piece); i += byteLength {
		if i > len(piece) {
			i = len(piece)
		}
		encodedString := hex.EncodeToString([]byte(piece[i : i+byteLength]))
		hashes = append(hashes, encodedString)
	}
	return hashes
}

func bencoder(data map[string]interface{}) string {
	index := 1
	encodedValue := "d"
	for k, v := range data {
		if len(data) < index {
			break
		}
		if _, ok := v.(int); ok {
			encodedValue = fmt.Sprintf("%s%d:%si%de", encodedValue, len(k), k, v.(int))
		} else {
			encodedValue = fmt.Sprintf("%s%d:%s%d:%s", encodedValue, len(k), k, len(v.(string)), v)
		}
		index += 1
	}
	encodedValue = fmt.Sprintf("%se", encodedValue)
	return encodedValue

}

func calculateHash(value string) string {
	h := sha1.New()
	h.Write([]byte(value))
	hash := h.Sum(nil)
	return fmt.Sprintf("%x", hash)
}
func printJson(value interface{}) {
	jsonEncoded, _ := json.MarshalIndent(value, "", "")
	fmt.Println(string(jsonEncoded))
}
func decodeBencodedInput(encoded string) map[string]interface{} {
	for i, v := range encoded {
		if i == 0 && v == 'd' {
			decodedValue, _ := decodeBencodedDict(encoded)
			return decodedValue
		}
		if unicode.IsDigit(v) && i == 0 {
			decodedValue, _, _ := decodeBencodedString(encoded)
			printJson(decodedValue)
			break
		}
		if i == 0 && v == 'i' {
			decodedValue, _, _ := decodeBencodedInteger(encoded)
			printJson(decodedValue)
			break
		}
		if i == 0 && v == 'l' {
			decodedValue, _ := decodeBencodedList(encoded)
			printJson(decodedValue)
			break
		}
	}
	return make(map[string]interface{}, 0)
}
func decodeBencodedString(encoded string) (string, int, error) {
	var actualString string
	var index int
	var totalLength int
	for i, v := range encoded {
		if unicode.IsDigit(v) && i == 0 {
			index = i
		}
		if v == ':' {
			length, err := strconv.Atoi(encoded[index:i])
			if err != nil {
				log.Fatalf("Failed converting bytes before ':' to int: %s", err)
			}
			actualString = encoded[i+1 : length+i+1]
			totalLength = len(encoded[index:i]) + len(actualString) + 1
			return actualString, totalLength, nil

		}
	}
	return actualString, totalLength, nil
}

func decodeBencodedInteger(encoded string) (int, int, error) {
	var length int
	var number string
	for k, v := range encoded {
		if v == 'e' {
			number = encoded[1:k]
			length = len(number) + 2
			break
		}
	}
	value, err := strconv.Atoi(number)
	return value, length, err
}
func decodeBencodedList(encoded string) ([]interface{}, int) {
	list := []interface{}{}
	offset := 1
	for offset < len(encoded)-1 {
		if encoded[offset] == 'e' {
			break
		}
		if encoded[offset] == 'd' {
			decodedValue, lengthForOffset := decodeBencodedDict(encoded[offset:])
			list = append(list, decodedValue)
			offset += lengthForOffset
			continue
		}
		if encoded[offset] == 'l' {
			decodedValue, lengthForOffset := decodeBencodedList(encoded[offset:])
			list = append(list, decodedValue)
			offset += lengthForOffset
			break
		}
		if currentRuneIsInt, decodedValue, lengthForOffset := IsParsebleString(encoded[offset], encoded[offset:]); currentRuneIsInt {
			list = append(list, decodedValue)
			offset += lengthForOffset
			continue
		}
		if isParsed, decodedValue, lengthForOffset := IsParsebleInt(encoded[offset], encoded[offset+1], encoded[offset:]); isParsed {
			list = append(list, decodedValue)
			offset += lengthForOffset
			continue
		} else {
			offset += lengthForOffset
			continue
		}
	}
	return list, len(encoded)
}
func decodeBencodedDict(encoded string) (map[string]interface{}, int) {
	dict := map[string]interface{}{}
	intoList, _ := decodeBencodedList(encoded)
	if len(intoList)%2 != 0 {
		log.Fatalf("Key must have a value")
	}
	for i := 0; i < len(intoList)-1; i += 2 {
		dict[intoList[i].(string)] = intoList[i+1]
	}
	_, err := json.MarshalIndent(dict, "", "")
	if err != nil {
		log.Fatalln(err)
	}
	return dict, len(encoded)
}
func IsParsebleString(currentByte byte, encodedSequence string) (bool, string, int) {
	if currentRuneIsInt := unicode.IsDigit(rune(currentByte)); currentRuneIsInt == true {
		decodedValue, encodedSequencelength, err := decodeBencodedString(encodedSequence)
		if err != nil {
			log.Fatalf("Failed decoding string in list - input : %s", err)
		}
		return true, decodedValue, encodedSequencelength
	}
	return false, "", 0
}
func IsParsebleInt(currentByte byte, nextByte byte, encodedSequence string) (bool, int, int) {
	if currentByte == 'i' {
		if nextByte != '-' {
			if _, err := strconv.Atoi(string(nextByte)); err != nil {
				log.Fatalf("Expected Integer after - : %s", err)
				return false, 0, 1
			}
		}
		decodedValue, encodedSequencelength, err := decodeBencodedInteger(encodedSequence)
		if err != nil {
			log.Fatalf("Failed decoding integer in list - input : %s", err)
		}
		return true, decodedValue, encodedSequencelength
	}
	return false, 0, 0
}
