### ffmpeg使用rtmp推流，收到的关键帧数据
ffmpeg -re -stream_loop -1 -i h264_30m59s.mp4 -vcodec copy -acodec copy -f flv rtmp://127.0.0.1/live/cctv0
第1个关键帧：video header
第2个关键帧：iframe
第3个关键帧：iframe

通过 video header得到 sps + pps
000001 674d401fe8802802dd80b501010140000003004000000c03c60c4480 000001 68ebef20

170100002a
00006050		24656
65888400fffe9ebbe05331604fb41fe0636feae75f23f8694e43107c8f840c0ee6a10278e034830f4797ca62e789fe1b232d
1d666f2be03090d254c17b8662537ef42fa6d91b74b18d1bf0a3ad762ee544d77103080c1b18586a5b66e32ceedf29316641
7a501f81362796a5f476580ebdf4fbbd93bec6343eaa737e4e6ac3f301b5f5528d8687987c5d8159caf80ee32838695f7461

1701000029
0000293b
6588840047fffea72fe053364504993cb8b0611b5fec23171240bb7a4e3181cc00e506e311718f3001edab76af9f5874c638
957e87b3bb964a40656bd3d9b09c4a456a80d8d8f1096a9667db61b092e442ebaafc92cab7679da45235fa526bfb93de39e7
22fc6214cb2baea8c26d6cceac4e361cd4239d47c22c7cd9d1e3d1f4dd324fc116909c2839808d3283e6a597537e9025021c

170100002a
000070d6
65888400bffe6d0bccb21d4b94645e01316359e871ad483ef346478b28bc07ea554d6145b3a1c1cb8ec99e1fd7f673e055bb
947d3904ec3781149cf1315c0939ffdd1b5be8267c3c9f66214df99ff1773e886add4e97ae183bb80ca0dd2ff199025a54ff
eb4177c071b944c63e93966f94d4ebaedb41268958fb4fad4e80c543f9e5fe48bacb66d6b101dcffff4e436db0d0e285837c


### ffmpeg使用rtmp推流，收到的关键帧数据
ffmpeg -re -stream_loop -1 -i fruit.mp4 -vcodec copy -acodec copy -f flv rtmp://127.0.0.1/live/cctv0
第1个关键帧：video header
第2个关键帧：iframe
第3个关键帧：iframe

000001 6764001facd1005005bb016a020202800001f480007530078c1889 000001 68eb8f2c

MsgLength=193565, MsgTypeId=9, DataType=VideoKeyFrame
1701000021
0002f414
25b840045ff456cbc75bbcfd906f0611899116f2a513ba8bf392ade7da04e477339ece221fb45e3ef5b9a29758ff632d05ca
67ca625ecb43b00605e1d0cc29039cac8e79f49909832f70f3e992d818d939a35c3930659946d354dc6344c897f9a5bba0d9
25e4a3077761da0317f16b7b493728a71236f09f71151d2e0ac0f4b724a75b07614b8a22d6bad442d1ee040dbf0721888c5d

1701000022
00030d78
25b840045ffce3cc10b5ef5e684c5f8da205be20d1b2dd9a20be8114789788dd7857fb99b462fb631f539aabb2e39919b501
64058ffecc2b11a3553102f9ea570816aec06caf89f53ecc513ca0ec50d63af1608c886ef3fb9dc3b22d0f11596ce75d8148
02d6031413238063b63409a06cd398710aa44ec880319fe704417753ab8a361e05f475686e61fc685e28ed0407de14252232

### ffmpeg使用rtmp推流，收到的关键帧数据
ffmpeg -re -stream_loop -1 -i h264_03m02s.mp4 -vcodec copy -acodec copy -f flv rtmp://127.0.0.1/live/cctv0
第1个关键帧：video header
第2个关键帧：sei + iframe
第3个关键帧：sei + iframe

000001 674d401fe8802802dd80b501010140000003004000000c03c60c4480 000001 68ebef20

170100002a
00000032
06052e			46
dc45e9bde6d948b7962cd820d923eeef78323634202d20636f72652031353520723239303120376430666632320080
00001c28		7208
65888400fffe9ebbe0514a043119ee3a01bfa7c08d3233fbc3f14108451d82de1fbf8fdf48f37c1ae5e2ab6b2ff3469ab753
e20795c8a0819fea06b13a5e44fcf881aa313caf41b08a88fef226e0ae73f86176fa5e0c4a5399c61066398185ef0cbbc1af
5df69a888166ad16454f04d4eaed6323648979cbf1156abbea52a814a72578400000030002a539f02afa6a94007d1e214d24
c3c619dc885afa393f16d75705376abca97f4d3bee76a69f7ac49d3cb9bb55e7d2b4e2eceb81ce15ee0ad1ea73250be4d3b5
1f9a68fc72f59aad026519f9eb32747fbecea8ef6

1701000029
00000032
06052e			46
dc45e9bde6d948b7962cd820d923eeef78323634202d20636f72652031353520723239303120376430666632320080
00003bf7
6588840067fffef5b17c0a6ac9f3319b86a8c2462bf0a9d7b17c04ff1f000003000003000003000003002d30887515909e34
07d800000301bdfe04c5a40351c2f41ace7960656779adf3411c40fdf5ba0c2cfacb3501493e0104e21beabbe3344a5f3dbb
de2ad31792a7f69e6d700c600b3dc00bc02ad1c1f398c48b74ddf72d3b9ae58673995ffa823583ec858ba9c4a0abb10b7ef1
53638f53bc7e3402d926186a6338da56d997cdddf4b917d8b6d90ea95156978db116645ab1a42214316afe8a7af135652f54


### obs使用rtmp推流，收到的关键帧数据
第1个关键帧：video header
第2个关键帧：sei + sps + pps + sei + iframe
第3个关键帧：sps + pps + iframe
第4个关键帧：sps + pps + iframe

000001 6764001facd9405005bb016a02020280000003008000001e478c18cb 000001 68efbcb0

1701000042
000002f6
0605fffff2	seiLen = 	255 + 255 + 242 = 752
dc45e9bde6d948b7962cd820d923eeef78323634202d20636f7265203136332072333035392062363834656265202d20482e
3236342f4d5045472d342041564320636f646563202d20436f70796c65667420323030332d32303231202d20687474703a2f
2f7777772e766964656f6c616e2e6f72672f783236342e68746d6c202d206f7074696f6e733a2063616261633d3120726566
3d31206465626c6f636b3d313a303a3020616e616c7973653d3078333a3078313133206d653d686578207375626d653d3220
7073793d31207073795f72643d312e30303a302e3030206d697865645f7265663d30206d655f72616e67653d313620636872
6f6d615f6d653d31207472656c6c69733d30203878386463743d312063716d3d3020646561647a6f6e653d32312c31312066
6173745f70736b69703d31206368726f6d615f71705f6f66667365743d3020746872656164733d3138206c6f6f6b61686561
645f746872656164733d3520736c696365645f746872656164733d30206e723d3020646563696d6174653d3120696e746572
6c616365643d3020626c757261795f636f6d7061743d3020636f6e73747261696e65645f696e7472613d3020626672616d65
733d3320625f707972616d69643d3220625f61646170743d3120625f626961733d30206469726563743d3120776569676874
623d31206f70656e5f676f703d3020776569676874703d31206b6579696e743d323530206b6579696e745f6d696e3d323520
7363656e656375743d343020696e7472615f726566726573683d302072635f6c6f6f6b61686561643d31302072633d636272
206d62747265653d3120626974726174653d313032342072617465746f6c3d312e302071636f6d703d302e36302071706d69
6e3d302071706d61783d3639207170737465703d34207662765f6d6178726174653d31303234207662765f62756673697a65	7
3d31303234206e616c5f6872643d6e6f6e652066696c6c65723d312069705f726174696f3d312e34302061713d313a312e30
300080
      0000001c 6764001facd9405005bb016a02020280000003008000001e478c18cb 00000004 68efbcb0
000002f6
0605fffff2
dc45e9bde6d948b7962cd820d923eeef78323634202d20636f7265203136332072333035392062363834656265202d20482e
3236342f4d5045472d342041564320636f646563202d20436f70796c65667420323030332d32303231202d20687474703a2f
2f7777772e766964656f6c616e2e6f72672f783236342e68746d6c202d206f7074696f6e733a2063616261633d3120726566
3d31206465626c6f636b3d313a303a3020616e616c7973653d3078333a3078313133206d653d686578207375626d653d3220
7073793d31207073795f72643d312e30303a302e3030206d697865645f7265663d30206d655f72616e67653d313620636872
6f6d615f6d653d31207472656c6c69733d30203878386463743d312063716d3d3020646561647a6f6e653d32312c31312066
6173745f70736b69703d31206368726f6d615f71705f6f66667365743d3020746872656164733d3138206c6f6f6b61686561
645f746872656164733d3520736c696365645f746872656164733d30206e723d3020646563696d6174653d3120696e746572
6c616365643d3020626c757261795f636f6d7061743d3020636f6e73747261696e65645f696e7472613d3020626672616d65
733d3320625f707972616d69643d3220625f61646170743d3120625f626961733d30206469726563743d3120776569676874
623d31206f70656e5f676f703d3020776569676874703d31206b6579696e743d323530206b6579696e745f6d696e3d323520
7363656e656375743d343020696e7472615f726566726573683d302072635f6c6f6f6b61686561643d31302072633d636272
206d62747265653d3120626974726174653d313032342072617465746f6c3d312e302071636f6d703d302e36302071706d69
6e3d302071706d61783d3639207170737465703d34207662765f6d6178726174653d31303234207662765f62756673697a65	7
3d31303234206e616c5f6872643d6e6f6e652066696c6c65723d312069705f726174696f3d312e34302061713d313a312e30
300080
	   0000c1b7 65888400df31af7b3c5f6b5a2ce303d9c93903528c600fa8dce0cd73b1a27450ff04b399938be432feb161
6348725cf1f5e55385c710a365c3bb419992e7e9284afe24b90d1e4e7e68c938e48741ae2362080180afe76451f722faa37d
4638d636ba1614a5f93035062e84a4c42a0bb3a298a30ca688774e13001151a0836aaeada5a9e18845e13


1701000042
0000001c 6764001facd9405005bb016a02020280000003008000001e478c18cb 00000004 68efbcb0
0000df6c
6588820019ff3db8fff50084a58b2d11d17335f40fe18139cc9f8d52777908ca09ab13a41e206fa6d60f63e6cf3fb9b79485
007f65e31c7f11f118aac21b747039f94c60cff72e561c705754f2e2241f3a4aa391329bb2642c8d8b717b1c293045d5e2a4
2cd544f06626c4af10feaf24f4732b2ff72998dcea65e18c598fc4c642b5a275a52f03b4cfa441d6e0a109040dbd231e69aa
72f867ede6f0a578c42801d28c7fe7cc9d68588e3dd982eba39091c479e9395e69299c1bc575a4f7ea54a893065abe3ff593


1701000042
0000001c 6764001facd9405005bb016a02020280000003008000001e478c18cb 00000004 68efbcb0
0000c852
65888403fff128c9fb49c9ff82c5fc123c37485464ad4758e644ebcc46a61b01906bf3d07d67d4c9fc6b04c444bc44c023bf
0deeb1edb333e2bb8e6796edfbf70d001ec4a0b56580c1f8aa3b54709563ee96d6984724246eb8d5a46af7ec69fc68025ea7
38aa3eb86eac051f77391f032622b96b29413923cbd232e7af264320ec0d15baf62987709ecb78b381af9fa912c475218fa7
37d10f0a4f62516f51b8638c845fec28a9d37cbfb6fe5df2ec08b35aa51ae653e8d00b0d89cd109f7d2efc4222208505ac6d