package manager

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/trustwallet/assets-go-libs/image"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	libFile "github.com/trustwallet/assets-go-libs/file"
	"github.com/trustwallet/assets-go-libs/path"
	"github.com/trustwallet/assets-go-libs/validation/info"
	"github.com/trustwallet/assets-go-libs/validation/tokenlist"
	"github.com/trustwallet/go-primitives/asset"
	"github.com/trustwallet/go-primitives/coin"
	"github.com/trustwallet/go-primitives/types"

	"github.com/trustwallet/assets/internal/config"
)

func CreateAssetInfoJSONTemplate(token string) error {
	c, tokenID, err := asset.ParseID(token)
	if err != nil {
		return fmt.Errorf("failed to parse token id: %v", err)
	}

	chain, ok := coin.Coins[c]
	if !ok {
		return fmt.Errorf("invalid token")
	}

	assetInfoPath := path.GetAssetInfoPath(chain.Handle, tokenID)

	var emptyStr string
	var emptyInt int
	assetInfoModel := info.AssetModel{
		Name:     &emptyStr,
		Type:     &emptyStr,
		Symbol:   &emptyStr,
		Decimals: &emptyInt,
		Website:  &emptyStr,
		Explorer: &emptyStr,
		Status:   &emptyStr,
		ID:       &tokenID,
		Links: []info.Link{
			{
				Name: &emptyStr,
				URL:  &emptyStr,
			},
		},
		Tags: []string{""},
	}

	bytes, err := json.Marshal(&assetInfoModel)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}

	f, err := libFile.CreateFileWithPath(assetInfoPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(f)

	_, err = f.Write(bytes)
	if err != nil {
		return fmt.Errorf("failed to write bytes to file")
	}

	err = libFile.FormatJSONFile(assetInfoPath)
	if err != nil {
		return fmt.Errorf("failed to format json file")
	}

	return nil
}

func AddTokenToTokenListJSON(chain coin.Coin, assetID, tokenID string, tokenListType path.TokenListType) error {
	setup()

	// Check for duplicates.
	tokenListTypes := []path.TokenListType{path.TokenlistDefault, path.TokenlistExtended}
	for _, t := range tokenListTypes {
		tokenListPath := path.GetTokenListPath(chain.Handle, t)
		var list tokenlist.Model

		err := libFile.ReadJSONFile(tokenListPath, &list)
		if err != nil {
			return fmt.Errorf("failed to read data from %s: %w", tokenListPath, err)
		}

		for _, item := range list.Tokens {
			if item.Asset == assetID {
				return fmt.Errorf("duplicate asset, already exist in %s", tokenListPath)
			}
		}
	}

	var list tokenlist.Model
	tokenListPath := path.GetTokenListPath(chain.Handle, tokenListType)

	err := libFile.ReadJSONFile(tokenListPath, &list)
	if err != nil {
		return fmt.Errorf("failed to read data from %s: %w", tokenListPath, err)
	}

	assetInfo, err := getAssetInfo(chain, tokenID)
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}

	newToken := tokenlist.Token{
		Asset:    assetID,
		Type:     types.TokenType(*assetInfo.Type),
		Address:  *assetInfo.ID,
		Name:     *assetInfo.Name,
		Symbol:   *assetInfo.Symbol,
		Decimals: uint(*assetInfo.Decimals),
		LogoURI:  path.GetAssetLogoURL(config.Default.URLs.AssetsApp, chain.Handle, tokenID),
	}
	list.Tokens = append(list.Tokens, newToken)

	return libFile.CreateJSONFile(tokenListPath, &tokenlist.Model{
		Name:      fmt.Sprintf("Trust Wallet: %s", coin.Coins[chain.ID].Name),
		LogoURI:   config.Default.URLs.Logo,
		Timestamp: time.Now().Format(config.Default.TimeFormat),
		Tokens:    list.Tokens,
		Version:   tokenlist.Version{Major: list.Version.Major + 1},
	})
}

func getAssetInfo(chain coin.Coin, tokenID string) (*info.AssetModel, error) {
	assetInfoPath := path.GetAssetInfoPath(chain.Handle, tokenID)
	var assetModel info.AssetModel

	err := libFile.ReadJSONFile(assetInfoPath, &assetModel)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from info.json: %w", err)
	}

	return &assetModel, nil
}

type RemoteAsset struct {
	ChainID       int    `json:"chainId"`
	Address       string `json:"address"`
	Name          string `json:"name"`
	Symbol        string `json:"symbol"`
	Decimals      int    `json:"decimals"`
	LogoURI       string `json:"logoURI"`
	OriginLogoURI string `json:"originLogoURI,omitempty"`
}

func createLogo(assetLogoPath string, a RemoteAsset) error {
	err := libFile.CreateDirPath(assetLogoPath)
	if err != nil {
		return err
	}

	// TODO: also handle jpg image.
	return image.CreatePNGFromURL(a.OriginLogoURI, assetLogoPath)
}

func CreateAssetInfoJSONTemplateNew(chain coin.Coin, token RemoteAsset) error {
	assetInfoPath := path.GetAssetInfoPath(chain.Handle, token.Address)
	asseLogoPath := path.GetAssetLogoPath(chain.Handle, token.Address)

	var emptyStr string
	var defaultType = "coin"

	var assetInfoModel = info.AssetModel{
		Name:     &token.Name,
		Type:     &defaultType,
		Symbol:   &token.Symbol,
		Decimals: &token.Decimals,
		Website:  &emptyStr,
		Explorer: &emptyStr,
		Status:   &emptyStr,
		ID:       &token.Address,
		Links: []info.Link{
			{
				Name: &emptyStr,
				URL:  &emptyStr,
			},
		},
		Tags: []string{""},
	}
	bytes, err := json.Marshal(&assetInfoModel)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}

	if createLogoErr := createLogo(asseLogoPath, token); createLogoErr != nil {
		return createLogoErr
	}

	f, err := libFile.CreateFileWithPath(assetInfoPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(f)

	_, err = f.Write(bytes)
	if err != nil {
		return fmt.Errorf("failed to write bytes to file")
	}

	err = libFile.FormatJSONFile(assetInfoPath)
	if err != nil {
		return fmt.Errorf("failed to format json file")
	}

	return nil
}

func getSupportChains() map[uint]string {
	return map[uint]string{
		coin.ETHEREUM: "",
		coin.POLYGON:  "",
		coin.BINANCE:  "",
		coin.AURORA:   "",
	}
}

func handleAsyncTokenList(chain uint, url string) {
	client := http.Client{}
	//nolint
	req, err := http.NewRequest(http.MethodGet, url, nil)

	if err != nil {
		log.Fatal(err)
	}

	res, getErr := client.Do(req)

	if getErr != nil {
		log.Fatal(getErr)
	}

	if res.Body != nil {
		defer func(Body io.ReadCloser) {
			closeErr := Body.Close()
			if closeErr != nil {
				log.Fatal(closeErr)
			}
		}(res.Body)
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		log.Fatal(readErr)
	}

	var assets []RemoteAsset
	jsonErr := json.Unmarshal(body, &assets)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	for _, a := range assets {
		_, err := getAssetInfo(coin.Coins[chain], a.Address)
		if err != nil {
			err := CreateAssetInfoJSONTemplateNew(coin.Coins[chain], a)
			if err != nil {
				fmt.Println("Create file failed: ", a.Address, err)
			}
		} else {
			fmt.Println("Create info.json succeed for: ", a.Address)
		}
	}

	fmt.Println(len(assets))
}
