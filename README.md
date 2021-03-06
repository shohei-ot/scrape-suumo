scrape-suumo
=================

- 検索結果 URL を渡して物件情報のサマリーを出力します。
- ホームディレクトリ直下に `.cache` ディレクトリが無ければ作成し、取得データを JSON でキャッシュします。
- キャッシュがある場合、差分を抽出して出力します。
- cron で定期的に動かして Slack に流す事を想定しています。


## build

```sh
make build
```

Raspberry Pi 用

```sh
make build-armv7
```

## usage

```sh
Usage of ./scrape-suumo:
  -channel string
        slack channel name
  -no-slack
        not use slack
  -refresh
        refresh suumo cache
  -token string
        slack access token
  -url string
        <required> suumo url of the search result
```

example:

```sh
scrape-suumo \
  -url="https://suumo.jp/jj/chintai/ichiran/FR301FC001/?ar=030&bs=040&ta=13&sc=13113&cb=7.0&ct=15.0&et=15&md=02&md=03&md=04&md=05&md=06&cn=25&mb=40&mt=9999999&tc=0401303&tc=0400101&tc=0400104&tc=0400501&tc=0400502&tc=0400601&tc=0400301&shkr1=03&shkr2=03&shkr3=03&shkr4=03&fw2=&srch_navi=1"
```

result:

```sh
Apartments: 3
Total Rooms: 3
------------------------------------------------------------
物件名: 都営大江戸線 西新宿五丁目駅 5階建 築16年
所在地: 東京都渋谷区本町４
最寄り: 都営大江戸線/西新宿五丁目駅 歩9分, 京王新線/初台駅 歩12分, 東京メトロ丸ノ内線/中野坂上駅 歩14分
築年: 築16年
階建: 5階建
部屋:
    - 階: 4階
    - 間取り: 1SDK
    - 面積: 57.74m2
    - 家賃: 12.3万円
    - 管理費: -
    - 礼金: 12.3万円
    - 敷金: 12.3万円
    - URL: https://suumo.jp/chintai/jnc_000071605340/?bc=100268963384
------------------------------------------------------------
物件名: 京王新線 初台駅 15階建 築17年
所在地: 東京都渋谷区本町１
最寄り: 京王新線/初台駅 歩5分, 京王新線/幡ヶ谷駅 歩7分, 小田急線/代々木上原駅 歩14分
築年: 築17年
階建: 15階建
部屋:
    - 階: 13階
    - 間取り: 1LDK
    - 面積: 40.6m2
    - 家賃: 14.85万円
    - 管理費: 11500円
    - 礼金: -
    - 敷金: -
    - URL: https://suumo.jp/chintai/jnc_000045513682/?bc=100269007494
------------------------------------------------------------
物件名: 京王線 笹塚駅 6階建 築16年
所在地: 東京都渋谷区幡ヶ谷１
最寄り: 京王線/笹塚駅 歩5分, 京王新線/幡ヶ谷駅 歩6分,
築年: 築16年
階建: 6階建
部屋:
    - 階: 2階
    - 間取り: 1K
    - 面積: 40.04m2
    - 家賃: 12.8万円
    - 管理費: 4000円
    - 礼金: 12.8万円
    - 敷金: 12.8万円
    - URL: https://suumo.jp/chintai/jnc_000046674934/?bc=100268878872
```
