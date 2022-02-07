###################################################
### rtmp流生成ts 注意事项
###################################################
1 rtmp里的Metadata, ts不需要
2 rtmp里的VideoHeader里有sps和pps, ts需要 放到关键帧前面, sps和pps前面要加 nalu标识0x00000001
3 rtmp里的视频数据是es(h264)裸流, ts要封装成pes, 数据前面要加 nalu标识0x00000001
4 rtmp里的AudioHeader里有音频流相关的信息, ts需要用它来生成adts
5 rtmp里的音频数据是es(aac)裸流，ts要封装成adts
6 tsFile是有很多个tsPakcet组成的，tsPacket的固定大小是188字节
7 pat，pmt是要连续插入到ts流中的，并不是开始插入之后就完了。因为ts要保证从任何时候都可以开始播放。最好每4帧视频(这4帧H264可能被打包成很多的ts包，而不止4包)插入一次pat和pmt, sps pps 也一样
8 continuity_counter的增长相对于所有PID相同的包他们的增长是独立的，例如第一个PAT包其continuity_counter值是0，那么第二个PAT包的continuity_counter是1。  第一个PMT包其continuity_counter值是2，第二个PMT包其continuity_counter值是3。
9 aac不需要设置DTS，只要PTS就可以了,加上DTS可能还播放不了
10 如果h264的包大于65535的话,可以设置PES_packet_length为0,表示不指定PES包的长度,ISO/ICE 13818-1.pdf 49/174 有说明,这主要是当一帧H264的长度大于PES_packet_length(2个字节)能表示的最大长度65535 的时候分包的问题, PES_packet_length的长度为0之后, 即使该H264视频帧的长度大于65535个字节也不需要分多个PES包存放, 事实证明这样做是可以的, ipad可播放
11 aac打包成PES的时候，要想在ipad上播放必须设置PES_packet_length的长度，而视频可以设置为0，但是音频必须设置为正确的长度值，aac的长度不可能超过65535，所以也不可能导致PES_packet_length溢出。否则ipad播放不了。但是QQ影音可以播放。
11 对于视频来说PTS和DTS都需要，可以设置为一样的，如果只有PTS，没有DTS是不行的。可以将pts的值赋值给dts，pts_dts_flag的值应该设置为0x03
12 ipad不需要pcr都可以播放，可以将pmt头中的 PCR_PID设置为0x1fff 表示没有PCR ,参考ISO/ICE 13818-1.pdf 65 / 174,  在之后的所有关于adaptation_field中设置PCR_flag: 00 就可以了，并且在adaptation_field中也不需要写入pcr部分的值
13 在每一帧的视频帧被打包到pes的时候，其开头一定要加上 00 00 00 01 09 xx  这个nal。否则就有问题, 其中 xx 不能设置为00，其他都可以，推荐设置为 f0

###################################################
### 关于TS流中的填充字节
###################################################
//因为TS每一包要求是188个字节, 当不足188个字节的时候, 必须要补充到188个字节, 这就涉及到填充的问题。TS流中有2中不同的填充形式，
//<1>. 如果TS包中承载的是PSI数据(PAT,PMT等)，那么其填充是在该包的有效字节后面填充0xFF直到满足188个字节为止。 解码器会丢弃这些字节，具体说明参考 ISO_IEC 13818-1.pdf 60/174
//<2>. 如果TS包中承载的是PES数据(音视频数据)，那么当不足188个字节的时候，需要使用adaptation_field 这个域来填充，tsHeader中AdaptationFieldControl设置为0x3, 同时指定Adaptation中的AdaptationFieldLength指定填充多少字节，后面各种指示器都为 0x0


### ts介绍
TS：全称为MPEG2-TS。TS即 传输流"Transport Stream"的缩写, 又称 MTS、TS。它是分包发送的，每一个包长为188字节（还有192和204个字节的包）。包的结构为，包头为4个字节（第一个字节为0x47），负载为184个字节。在TS流里可以填入很多类型的数据，如视频、音频、自定义信息等。MPEG2-TS主要应用于实时传送的节目，比如实时广播的电视节目, 如 DVB、ATSC、IPTV 等。MPEG2-TS格式的特点就是要求从视频流的任一片段开始都是可以独立解码的。简单地说，将DVD上的VOB文件的前面一截cut掉（或者是数据损坏数据）就会导致整个文件无法解码，而电视节目是任何时候打开电视机都能解码（收看）的。
MPEG2-TS 定义于 MPEG-2 的 ISO/IEC 13818-1

### TS流包含的内容
一段TS流，必须包含PAT包、PMT包、多个音频包、多个视频包、多个PCR包、以及其他信息包。

TsFile 由 TsPacket 组成
TsPacket 包为188字节
Pes 被拆分装进 TsPacket
Pes 里面封装了 以Nalu为单元的Es数据
Es 就是 压缩编码后的 音视频数据, 如：h265/ aac

### 解析TS流数据的流程
查找PID为0x0的包，解析PAT，PAT包中的program_map_PID表示PMT的PID；查找PMT，PMT包中的elementary_PID表示音视频包的PID，PMT包中的PCR_PID表示PCR的PID，有的时候PCR的PID跟音频或者视频的PID相同，说明PCR会融进音视频的包，注意解析，有的时候PCR是自己单独的包；CAT、NIT、SDT、EIT的PID分别为: 0x01、0x10、0x11、0x12。

### PSI 与 PAT PMT
在MPEG-2中定义了节目特定信息PSI, PSI用来描述传送流的组成结构.
PSI信息由以下几种类型表组成
* 节目关联表（PAT Program Association Table）
* 节目映射表（PMT Program Map Table）
* 网络信息表（NIT Nerwork Information Table）
* 条件接收表（CAT Conditional Access Table）
* 传输流描述表（TSDT Transport Stream Description Table）
PAT表给出了一路MPEG-II码流中有多少套节目，以及它与PMT表PID之间的对应关系；
PMT表给出了一套节目的具体组成情况与其视频、音频等PID对应关系。

### PTS DTS PCR
PTS(Presentation Time Stamp) 显示时间戳, 在 PES 头信息里
DTS(Decoding Time Stamp) 解码时间戳, 在 PES 头信息里
PCR(Program Clock Reference) 节目时钟参考，在 tsHeader里的adaptation里，用于恢复出与编码端一致的系统时序时钟
SCR(system clock reference) 系统时钟参考
STC(System Time Clock) 系统时序时钟, 存在与编解码端

除了PTS和DTS的配合工作外，还有一个重要的参数是SCR(system clock reference)。在编码的时候，PTS，DTS和SCR都是由STC(system time clock)生成的，在解码时，STC会再生，并通过锁相环路（PLL－phase lock loop），用本地SCR相位与输入的瞬时SCR相位锁相比较，以确定解码过程是否同步，若不同步，则用这个瞬时SCR调整27MHz的本地时钟频率。最 后，PTS，DTS和SCR一起配合，解决视音频同步播放的问题

标准规定在原始音频和视频流中, PTS的间隔不能超过0.7s， 出现在TS包头的PCR间隔不能超过0.1s。

编码器
系统时钟STC: 编码器中有一个系统时钟(其频率是27MHz), 此时钟用来产生指示音视频的正确显示和解码的时间戳, 同时可用来指示在采样过程中系统时钟本身的瞬时值。
PCR(Program Clock Reference): 指示系统时钟本身的瞬时值的时间标签称为节目参考时钟标签(PCR)。 PCR的插入必须在PCR字段的最后离开复用器的那一时刻, 同时把27MHz系统时钟的采样瞬时值作为PCR字段插入到相应的PCR域。 它是放在TS包头的自适应区中传送.
27MHz的系统时钟STC经波形整理后分成两路:
PCR_ext (9bits ),   由27MHz脉冲直接触发计数器生成扩展域.
PCR_base(33bits), 经300分频器分频成90kHz脉冲送入一个33位计数器生成90kHz基值, 用于和PTS/DTS比较，产生解码和显示所需要的同步信号. 
这两部分被置入PCR域，共同组成42位的PCR.

### SPS PPS
SPS和PPS包含了初始化H.264解码器所需要的信息参数。
SPS包含的是针对一连续编码视频序列的参数，如标识符seq_parameter_set_id、帧数及POC的约束、参考帧数目、解码图像尺寸和帧场编码模式选择标识等。
PPS对应的是一个序列中某一副图像或者某几幅图像，参数如标识符pic_parameter_set_id、可选的seq_parameter_set_id、熵编码模式选择标识、片组数目、初始量化参数和去方块滤波系数调整标识等。
SPS即Sequence Paramater Set，又称作序列参数集。SPS中保存了一组编码视频序列(Coded video sequence)的全局参数。所谓的编码视频序列即原始视频的一帧一帧的像素数据经过编码之后的结构组成的序列。而每一帧的编码后数据所依赖的参数保存于图像参数集中。一般情况SPS和PPS的NAL Unit通常位于整个码流的起始位置。但在某些特殊情况下，在码流中间也可能出现这两种结构，主要原因可能为：
1 解码器需要在码流中间开始解码；
2 编码器在编码的过程中改变了码流的参数（如图像分辨率等）；
在做视频播放时，为了让后续的解码过程可以使用SPS中包含的参数，必须对其中的数据进行解析。
即认为SPS和PPS都是特殊的NALU。一个MP4文件只有一个SPS，但是有很多PPS，SPS必须在所有NALU的最开头。
