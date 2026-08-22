package casimport

import (
	"fmt"
	"testing"
)

var fullCleanDocPages = []string{
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
PAN: APPPR4110Q
SABYASACHI ROY
4669, DRUMMOND DRIVEVANCOUVERVANCOUVER - 0, BRITISH CO, CANADAMobile: 17783256624Email: SABYASACH2@GMAIL.COM
The Consolidated Account Statement is brought to you as an investor friendly initiative byCAMS and KFintech, and list the transactions, balances and valuation of Mutual Funds in which youare holding investments. The consolidation has been carried out based on your PAN.If you find any folios missing in this consolidation, please check if your PAN is updated across all yourMutual Fund folios.
Page 1 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Allocation by Asset Class
83.78%
16.22%
EQUITY
FOF`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 2 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 3 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
Nippon India Mutual Fund
FOLIO NO: 499388482035
NIPPON INDIA GROWTH MID CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION (Advisor: /DIRECT)  ISIN: INF204K01E54
KYC : OK
Opening Unit Balance: 0.000
01-JUL-2025
Purchase Trxn.Ref.No.pay_QnjoMAgGPZYGW0//Icici Bank Limited -036001076406//netbanking
24,998.75
5.409
4,621.60
5.409
08-JUL-2025
Purchase Trxn.Ref.No.pay_QqVpg1ZAdY7AL1//Icici Bank Limited -036001076406//netbanking
24,998.75
5.475
4,565.72
10.884
15-JUL-2025
Purchase Trxn.Ref.No.pay_QtHc9C3t4vb5Na//Icici Bank Limited -036001076406//netbanking
24,998.75
5.443
4,593.01
16.327
22-JUL-2025
Purchase Trxn.Ref.No.pay_Qw1M0xnMQUHc4a//Icici Bank Limited -036001076406//netbanking
24,998.75
5.447
4,589.47
21.774
25-JUL-2025
Purchase Trxn.Ref.No.pay_QxFQyomMlOlGaQ//Icici Bank Limited -036001076406//netbanking
24,998.75
5.514
4,533.50
27.288
29-JUL-2025
Purchase Trxn.Ref.No.pay_QypZQJvlS62oYu//Icici Bank Limited -036001076406//netbanking
24,998.75
5.519
4,529.58
32.807
06-AUG-2025
Purchase Trxn.Ref.No.pay_R1zmh9lgcPZyiG//Icici Bank Limited -036001076406//netbanking
24,998.75
5.617
4,450.86
38.424
21-AUG-2025
Purchase Trxn.Ref.No.pay_R7vmZmrzF6FIOu//Icici Bank Limited -036001076406//netbanking
24,998.75
5.450
4,586.81
43.874
26-AUG-2025
Purchase Trxn.Ref.No.pay_R9tpK9ZvjZxSFm//Icici Bank Limited -036001076406//netbanking
24,998.75
5.548
4,506.17
49.422
05-SEP-2025
Purchase Trxn.Ref.No.pay_RDqyDYlRRxgUcI//Icici Bank Limited -036001076406//netbanking
24,998.75
5.547
4,506.71
54.969
09-SEP-2025
Purchase Trxn.Ref.No.pay_RFRl5rLPMJ0Kcw-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
5.520
4,528.70
60.489
17-SEP-2025
Purchase Trxn.Ref.No.pay_RIbOhLXOHuJRon//Icici Bank Limited - 036001076406//
24,998.75
5.386
4,641.10
65.875`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 4 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
17-SEP-2025
netbanking
24,998.75
5.386
4,641.10
65.875
23-SEP-2025
Purchase Trxn.Ref.No.pay_RKzKFNNTqCoycJ//Icici Bank Limited -036001076406//netbanking
24,998.75
5.418
4,613.80
71.293
26-SEP-2025
Purchase Trxn.Ref.No.pay_RMAbM5vQMJWkZt//Icici Bank Limited -036001076406//netbanking
49,997.50
11.186
4,469.85
82.479
03-OCT-2025
Purchase Trxn.Ref.No.pay_ROuUuK5qjRgLB9//Icici Bank Limited -036001076406//netbanking
24,998.75
5.502
4,543.45
87.981
08-OCT-2025
Purchase Trxn.Ref.No.pay_RQuGseGYkUNMrJ//Icici Bank Limited -036001076406//netbanking
24,998.75
5.484
4,558.51
93.465
14-OCT-2025
Purchase Trxn.Ref.No.pay_RTH7R9rRVweHeE//Icici Bank Limited -036001076406//netbanking
24,998.75
5.460
4,578.52
98.925
23-OCT-2025
Purchase Trxn.Ref.No.pay_RWqSDnV6MsCTO1//Icici Bank Limited -036001076406//netbanking
24,998.75
5.359
4,664.86
104.284
28-OCT-2025
Purchase Trxn.Ref.No.pay_RYpNVyqNlGhe9J//Icici Bank Limited -036001076406//netbanking
24,998.75
5.350
4,672.77
109.634
04-NOV-2025
Purchase Trxn.Ref.No.pay_RbayYi51pwzg7j//Icici Bank Limited -036001076406//netbanking
24,998.75
5.338
4,682.90
114.972
12-NOV-2025
Purchase Trxn.Ref.No.pay_RekPRoconRFbzE//Icici Bank Limited -036001076406//netbanking
24,998.75
5.319
4,699.72
120.291
19-NOV-2025
Purchase Trxn.Ref.No.pay_RhVRWzJ9nWxltA-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
5.316
4,702.70
125.607
22-JAN-2026
Purchase Trxn.Ref.No.pay_S6pXkvSW3SD8CN//Icici Bank Limited -036001076406//netbanking
24,998.75
5.536
4,515.63
131.143
27-JAN-2026
Purchase Trxn.Ref.No.pay_S8psPeLIi76c0d//Icici Bank Limited -036001076406//netbanking
24,998.75
5.606
4,459.68
136.749
16-MAR-2026
Purchase
24,998.75
5.700
4,385.99
142.449`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 5 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
23-MAR-2026
Purchase Trxn.Ref.No.pay_SUakRGLq8qaDyb-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
5.930
4,215.88
148.379
Closing Unit Balance: 148.379
Nav as on 07-AUG-2026: INR 5,049.2657
Valuation on 09-Aug-2026 : INR 7,49,205.00`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 6 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA INDEX FUND - NIFTY 50 PLAN - DIRECT GROWTH PLAN GROWTH OPTION (Advisor: /DIRECT)  ISIN: INF204K01H36
KYC : OK
Opening Unit Balance: 0.000
14-JAN-2025
Sys. Investment Trxn.Ref.No.pay_PjFcLpqzHGeDgZ//ICICI BANK LIMITED -036001076406/netbanking (1/28)
24,998.75
595.435
41.98
595.435
20-JAN-2025
Purchase Trxn.Ref.No.pay_Plc81enOB1BOk4//Icici Bank Limited -036001076406//netbanking
99,995.00
2,362.423
42.33
2,957.858
27-JAN-2025
Purchase Trxn.Ref.No.pay_PoNeXsKiToD2Zc//Icici Bank Limited -036001076406//netbanking
99,995.00
2,415.752
41.39
5,373.610
30-JAN-2025
Purchase Trxn.Ref.No.pay_PpDK1NDiHt1OID//Icici Bank Limited -036001076406//netbanking
24,998.75
592.973
42.16
5,966.583
30-JAN-2025
Purchase Trxn.Ref.No.pay_PpYhjhlcvx1wLr//Icici Bank Limited -036001076406//netbanking
2,49,987.50
5,929.734
42.16
11,896.317
03-FEB-2025
Purchase Trxn.Ref.No.pay_PrAqsXXc8DDU71//Icici Bank Limited -036001076406//netbanking
2,49,987.50
5,899.995
42.37
17,796.312
10-FEB-2025
Sys. Investment ISIP (2/28)
24,998.75
589.226
42.43
18,385.538
14-FEB-2025
Purchase Trxn.Ref.No.pay_PvUqJkpzjZz3kf//Icici Bank Limited -036001076406//netbanking
1,24,993.75
3,002.420
41.63
21,387.958
17-FEB-2025
Sys. Investment ISIP (3/28)
24,998.75
599.699
41.69
21,987.657
24-FEB-2025
Sys. Investment ISIP (4/28)
24,998.75
610.494
40.95
22,598.151
03-MAR-2025
Sys. Investment ISIP (5/28)
24,998.75
622.459
40.16
23,220.610
10-MAR-2025
Sys. Investment ISIP (6/28)
24,998.75
613.041
40.78
23,833.651`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 7 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
17-MAR-2025
Sys. Investment ISIP (7/28)
24,998.75
611.710
40.87
24,445.361
24-MAR-2025
Sys. Investment ISIP (8/28)
24,998.75
582.025
42.95
25,027.386
01-APR-2025
Sys. Investment ISIP (9/28)
24,998.75
594.420
42.06
25,621.806
08-APR-2025
Sys. Investment ISIP (10/28)
24,998.75
611.115
40.91
26,232.921
15-APR-2025
Sys. Investment ISIP (11/28)
24,998.75
590.376
42.34
26,823.297
22-APR-2025
Sys. Investment ISIP (12/28)
24,998.75
569.923
43.86
27,393.220
02-MAY-2025
Sys. Investment ISIP (13/28)
24,998.75
565.638
44.20
27,958.858
08-MAY-2025
Sys. Investment ISIP (14/28)
24,998.75
567.342
44.06
28,526.200
15-MAY-2025
Sys. Investment ISIP (15/28)
24,998.75
549.500
45.49
29,075.700
22-MAY-2025
Sys. Investment ISIP (16/28)
24,998.75
559.300
44.70
29,635.000
02-JUN-2025
Sys. Investment ISIP (17/28)
24,998.75
556.059
44.96
30,191.059
09-JUN-2025
Sys. Investment ISIP (18/28)
24,998.75
547.119
45.69
30,738.178
16-JUN-2025
Sys. Investment ISIP (19/28)
24,998.75
550.479
45.41
31,288.657
23-JUN-2025
Sys. Investment ISIP (20/28)
24,998.75
549.822
45.47
31,838.479`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 8 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
01-JUL-2025
Sys. Investment ISIP (21/28)
24,998.75
536.779
46.57
32,375.258
08-JUL-2025
Sys. Investment ISIP (22/28)
24,998.75
537.004
46.55
32,912.262
15-JUL-2025
Sys. Investment ISIP (23/28)
24,998.75
543.911
45.96
33,456.173
22-JUL-2025
Sys. Investment ISIP (24/28)
24,998.75
546.496
45.74
34,002.669
25-JUL-2025
Purchase Trxn.Ref.No.pay_QxF6QEZTQoxRmw//Icici Bank Limited -036001076406//netbanking
24,998.75
551.138
45.36
34,553.807
01-AUG-2025
Sys. Investment ISIP (25/28)
24,998.75
557.007
44.88
35,110.814
08-AUG-2025
Sys. Investment ISIP (26/28)
24,998.75
561.529
44.52
35,672.343
18-AUG-2025
Sys. Investment ISIP (27/28)
24,998.75
549.363
45.51
36,221.706
22-AUG-2025
Sys. Investment ISIP (28/28)
24,998.75
549.435
45.50
36,771.141
Closing Unit Balance: 36,771.141
Nav as on 07-AUG-2026: INR 45.3849
Valuation on 09-Aug-2026 : INR 16,68,854.56`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 9 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA MULTI CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION (Advisor: /DIRECT)  ISIN: INF204K01XF9
KYC : OK
Opening Unit Balance: 0.000
18-MAR-2026
Purchase Trxn.Ref.No.pay_SScvmA9R4A0ns1-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
49,997.50
160.073
312.34
160.073
23-MAR-2026
Purchase Trxn.Ref.No.pay_SUaeIwY1YzxG7O-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
49,997.50
169.642
294.72
329.715
09-APR-2026
Purchase Trxn.Ref.No.pay_SbJphJKz700nMK-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
49,997.50
159.859
312.76
489.574
16-APR-2026
Purchase Trxn.Ref.No.pay_Se6arIUH3Ayzqt-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
49,997.50
154.408
323.80
643.982
22-APR-2026
Purchase Trxn.Ref.No.pay_SgSIi5CjJ5VzBe-NA-NETBANKING//Icici Bank Limited -036001076406/dc
49,997.50
151.041
331.02
795.023
29-APR-2026
Purchase Trxn.Ref.No.pay_SjCIcSUxfJX23n-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
49,997.50
151.972
328.99
946.995
Closing Unit Balance: 946.995
Nav as on 07-AUG-2026: INR 340.2024
Valuation on 09-Aug-2026 : INR 3,22,169.97`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 10 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA NIFTY 50 VALUE 20 INDEX FUND - DIRECT GROWTH PLAN (Advisor: /DIRECT)  ISIN: INF204KB12Z0
KYC : OK
Opening Unit Balance: 0.000
24-JAN-2025
Sys. Investment Trxn.Ref.No.pay_PnBMxNQYJRpgLV//ICICI BANK LIMITED -036001076406/netbanking (1/27)
24,998.75
1,304.716
19.16
1,304.716
30-JAN-2025
Purchase Trxn.Ref.No.pay_PpYwZ9fLn60GVT//Icici Bank Limited -036001076406//netbanking
1,49,992.50
7,856.095
19.09
9,160.811
03-FEB-2025
Purchase Trxn.Ref.No.pay_PrAwhfjMsava1N//Icici Bank Limited -036001076406//netbanking
2,49,987.50
13,091.435
19.10
22,252.246
14-FEB-2025
Purchase Trxn.Ref.No.pay_PvUv7s1zDdPrzY//Icici Bank Limited -036001076406//netbanking
74,996.25
4,032.360
18.60
26,284.606
27-FEB-2025
Purchase Trxn.Ref.No.pay_Q0ebf44vlTgkaY//Icici Bank Limited -036001076406//netbanking
24,998.75
1,387.632
18.02
27,672.238
03-MAR-2025
Sys. Investment ISIP (2/27)
24,998.75
1,418.667
17.62
29,090.905
10-MAR-2025
Sys. Investment ISIP (3/27)
24,998.75
1,396.375
17.90
30,487.280
17-MAR-2025
Sys. Investment ISIP (4/27)
24,998.75
1,409.222
17.74
31,896.502
24-MAR-2025
Sys. Investment ISIP (5/27)
24,998.75
1,346.132
18.57
33,242.634
01-APR-2025
Sys. Investment ISIP (6/27)
24,998.75
1,380.803
18.10
34,623.437
08-APR-2025
Sys. Investment ISIP (7/27)
24,998.75
1,434.558
17.43
36,057.995
15-APR-2025
Sys. Investment ISIP (8/27)
24,998.75
1,402.360
17.83
37,460.355`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 11 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
22-APR-2025
Sys. Investment ISIP (9/27)
24,998.75
1,367.793
18.28
38,828.148
02-MAY-2025
Sys. Investment ISIP (10/27)
24,998.75
1,349.198
18.53
40,177.346
08-MAY-2025
Sys. Investment ISIP (11/27)
24,998.75
1,355.284
18.45
41,532.630
15-MAY-2025
Sys. Investment ISIP (12/27)
24,998.75
1,306.530
19.13
42,839.160
22-MAY-2025
Sys. Investment ISIP (13/27)
24,998.75
1,333.437
18.75
44,172.597
02-JUN-2025
Sys. Investment ISIP (14/27)
24,998.75
1,331.038
18.78
45,503.635
09-JUN-2025
Sys. Investment ISIP (15/27)
24,998.75
1,316.978
18.98
46,820.613
16-JUN-2025
Sys. Investment ISIP (16/27)
24,998.75
1,313.153
19.04
48,133.766
23-JUN-2025
Sys. Investment ISIP (17/27)
24,998.75
1,326.595
18.84
49,460.361
01-JUL-2025
Sys. Investment ISIP (18/27)
24,998.75
1,311.919
19.06
50,772.280
08-JUL-2025
Sys. Investment ISIP (19/27)
24,998.75
1,307.760
19.12
52,080.040
15-JUL-2025
Sys. Investment ISIP (20/27)
24,998.75
1,325.800
18.86
53,405.840
22-JUL-2025
Sys. Investment ISIP (21/27)
24,998.75
1,330.705
18.79
54,736.545
25-JUL-2025
Purchase Trxn.Ref.No.pay_QxFFdcCNoHAyu0//Icici Bank Limited -036001076406//netbanking
24,998.75
1,345.067
18.59
56,081.612`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 12 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
01-AUG-2025
Sys. Investment ISIP (22/27)
24,998.75
1,367.016
18.29
57,448.628
08-AUG-2025
Sys. Investment ISIP (23/27)
24,998.75
1,374.199
18.19
58,822.827
18-AUG-2025
Sys. Investment ISIP (24/27)
24,998.75
1,346.814
18.56
60,169.641
22-AUG-2025
Sys. Investment ISIP (25/27)
24,998.75
1,343.419
18.61
61,513.060
01-SEP-2025
Sys. Investment ISIP (26/27)
24,998.75
1,337.146
18.70
62,850.206
08-SEP-2025
Sys. Investment ISIP (27/27)
24,998.75
1,346.125
18.57
64,196.331
Closing Unit Balance: 64,196.331
Nav as on 07-AUG-2026: INR 18.0897
Valuation on 09-Aug-2026 : INR 11,61,292.37`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 13 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA NIFTY 500 MOMENTUM 50 INDEX FUND - DIRECT GROWTH PLAN (Advisor: /DIRECT)  ISIN: INF204KC1DG5
KYC : OK
Opening Unit Balance: 0.000
20-FEB-2025
Purchase Trxn.Ref.No.pay_PxuOURlPC4l6K5//Icici Bank Limited -036001076406//netbanking
24,998.75
3,310.917
7.55
3,310.917
21-FEB-2025
Purchase Trxn.Ref.No.pay_PyIb9RTMClwoIS//Icici Bank Limited -036001076406//netbanking
74,996.25
10,100.097
7.43
13,411.014
27-FEB-2025
Purchase Trxn.Ref.No.pay_Q0eUyouy3ac08c//Icici Bank Limited -036001076406//netbanking
49,997.50
6,932.735
7.21
20,343.749
10-MAR-2025
Purchase Trxn.Ref.No.pay_Q50GUHv6zzbec3//Icici Bank Limited -036001076406//netbanking
49,997.50
7,124.891
7.02
27,468.640
19-MAR-2025
Purchase Trxn.Ref.No.pay_Q8Zjb3o5uB5gRV//Icici Bank Limited -036001076406//netbanking
24,998.75
3,411.355
7.33
30,879.995
21-MAR-2025
Purchase Trxn.Ref.No.pay_Q9Mw7txo3V2isV//Icici Bank Limited -036001076406//netbanking
49,997.50
6,694.540
7.47
37,574.535
25-MAR-2025
Purchase Trxn.Ref.No.pay_QAvFTotp3bFdlG//Icici Bank Limited -036001076406//netbanking
49,997.50
6,714.499
7.45
44,289.034
01-APR-2025
Purchase Trxn.Ref.No.pay_QDiZJU6U8v9utM//Icici Bank Limited -036001076406//netbanking
24,998.75
3,410.005
7.33
47,699.039
04-APR-2025
Purchase Trxn.Ref.No.pay_QEtFz8GHBynuKF//Icici Bank Limited -036001076406//netbanking
24,998.75
3,496.720
7.15
51,195.759
07-APR-2025
Purchase Trxn.Ref.No.pay_QG5hkJcRigk1gt//Icici Bank Limited -036001076406//netbanking
24,998.75
3,629.530
6.89
54,825.289
21-APR-2025
Purchase Trxn.Ref.No.pay_QLclY8BGi2yZ9E//Icici Bank Limited -036001076406//netbanking
49,997.50
6,513.059
7.68
61,338.348
29-APR-2025
Purchase Trxn.Ref.No.pay_QOo6pxnNzKXn2U-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
3,188.901
7.84
64,527.249`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 14 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
06-MAY-2025
Purchase Trxn.Ref.No.pay_QRam6oAi7pMPMi//Icici Bank Limited -036001076406//netbanking
49,997.50
6,466.978
7.73
70,994.227
09-MAY-2025
Purchase Trxn.Ref.No.pay_QSldTOtgN0voqY//Icici Bank Limited -036001076406//netbanking
24,998.75
3,269.306
7.65
74,263.533
14-MAY-2025
Purchase Trxn.Ref.No.pay_QUhph3oKi3ubrs//Icici Bank Limited -036001076406//netbanking
24,998.75
3,089.278
8.09
77,352.811
16-MAY-2025
Purchase Trxn.Ref.No.pay_QVXQyAur26E8xL//Icici Bank Limited -036001076406//netbanking
24,998.75
3,050.600
8.19
80,403.411
20-MAY-2025
Purchase  pay_QX7rFHuCeYwBBU netbanking ICIC0000360 Icici BankLimited036001076406
24,998.75
3,100.966
8.06
83,504.377
22-MAY-2025
Sys. Investment ISIP (1/14)
12,499.38
1,548.256
8.07
85,052.633
26-MAY-2025
Purchase Trxn.Ref.No.pay_QZUFDx51MbYcgy//Icici Bank Limited -036001076406//netbanking
24,998.75
3,060.534
8.17
88,113.167
02-JUN-2025
Sys. Investment ISIP (2/14)
12,499.38
1,524.426
8.20
89,637.593
03-JUN-2025
Purchase Trxn.Ref.No.pay_QcfAvhOH3q4Usd//Icici Bank Limited -036001076406//netbanking
24,998.75
3,045.211
8.21
92,682.804
09-JUN-2025
Purchase Trxn.Ref.No.pay_Qf0r1r3DkydHRv//Icici Bank Limited -036001076406//netbanking
87,495.63
10,235.442
8.55
1,02,918.246
09-JUN-2025
Sys. Investment ISIP (3/14)
12,499.38
1,462.207
8.55
1,04,380.453
12-JUN-2025
Purchase Trxn.Ref.No.pay_QgE8JdfZwY7nb0//Icici Bank Limited -036001076406//netbanking
24,998.75
2,980.406
8.39
1,07,360.859
16-JUN-2025
Sys. Investment ISIP (4/14)
12,499.38
1,478.482
8.45
1,08,839.341
17-JUN-2025
Purchase Trxn.Ref.No.pay_QiCWfJ2iPVbUti//Icici Bank Limited -036001076406//netbanking
37,498.13
4,451.239
8.42
1,13,290.580`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 15 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
23-JUN-2025
Sys. Investment ISIP (5/14)
12,499.38
1,471.623
8.49
1,14,762.203
26-JUN-2025
Purchase Trxn.Ref.No.pay_QlkJmN9tUcf73A//Icici Bank Limited -036001076406//netbanking
12,499.38
1,449.304
8.62
1,16,211.507
30-JUN-2025
Purchase Trxn.Ref.No.pay_QnLgszDXpXyEss//Icici Bank Limited -036001076406//netbanking
24,998.75
2,900.693
8.62
1,19,112.200
01-JUL-2025
Sys. Investment ISIP (6/14)
12,499.38
1,456.431
8.58
1,20,568.631
03-JUL-2025
Purchase Trxn.Ref.No.pay_QoXBTSCzZMYMvO//Icici Bank Limited -036001076406//netbanking
24,998.75
2,948.383
8.48
1,23,517.014
08-JUL-2025
Sys. Investment ISIP (7/14)
12,499.38
1,486.287
8.41
1,25,003.301
11-JUL-2025
Purchase Trxn.Ref.No.pay_QrgmubljksBd6m//Icici Bank Limited -036001076406//netbanking
24,998.75
3,014.549
8.29
1,28,017.850
15-JUL-2025
Sys. Investment ISIP (8/14)
12,499.38
1,493.908
8.37
1,29,511.758
17-JUL-2025
Purchase Trxn.Ref.No.pay_Qu5Bw3EPeHULHh//Icici Bank Limited -036001076406//netbanking
12,499.38
1,503.414
8.31
1,31,015.172
22-JUL-2025
Sys. Investment ISIP (9/14)
12,499.38
1,499.536
8.34
1,32,514.708
24-JUL-2025
Purchase
24,998.75
3,017.059
8.29
1,35,531.767
25-JUL-2025
Purchase Trxn.Ref.No.pay_QxF9aArPkoBHS1//Icici Bank Limited -036001076406//netbanking
24,998.75
3,054.514
8.18
1,38,586.281
28-JUL-2025
Purchase Trxn.Ref.No.pay_QyR0u3ErQEzuBe//Icici Bank Limited -036001076406//netbanking
24,998.75
3,094.058
8.08
1,41,680.339
01-AUG-2025
Sys. Investment ISIP (10/14)
12,499.38
1,566.868
7.98
1,43,247.207`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 16 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
08-AUG-2025
Sys. Investment ISIP (11/14)
12,499.38
1,584.627
7.89
1,44,831.834
18-AUG-2025
Sys. Investment ISIP (12/14)
12,499.38
1,539.105
8.12
1,46,370.939
22-AUG-2025
Purchase Trxn.Ref.No.pay_R8IyQIDb1HyqSh//Icici Bank Limited -036001076406//netbanking
12,499.38
1,538.272
8.13
1,47,909.211
22-AUG-2025
Sys. Investment ISIP (13/14)
12,499.38
1,538.272
8.13
1,49,447.483
26-AUG-2025
Purchase Trxn.Ref.No.pay_R9u1MZXGkA5Upu//Icici Bank Limited -036001076406//netbanking
12,499.38
1,567.183
7.98
1,51,014.666
01-SEP-2025
Sys. Investment ISIP (14/14)
12,499.38
1,573.041
7.95
1,52,587.707
27-JAN-2026
Purchase Trxn.Ref.No.pay_S8pwWcN7zld7lY//Icici Bank Limited -036001076406//netbanking
24,998.75
3,180.422
7.86
1,55,768.129
23-MAR-2026
Purchase Trxn.Ref.No.pay_SUaogIffks4FSN-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
3,496.035
7.15
1,59,264.164
Closing Unit Balance: 1,59,264.164
Nav as on 07-AUG-2026: INR 8.5294
Valuation on 09-Aug-2026 : INR 13,58,427.76`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 17 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA NIFTY MIDCAP 150 INDEX FUND - DIRECT GROWTH PLAN (Advisor: /DIRECT)  ISIN: INF204KB18Z7
KYC : OK
Opening Unit Balance: 0.000
19-MAR-2025
Purchase Trxn.Ref.No.pay_Q8ZrpuT5qDz6mG//Icici Bank Limited -036001076406//netbanking
24,998.75
1,149.680
21.74
1,149.680
21-MAR-2025
Purchase Trxn.Ref.No.pay_Q9N8XYKZAnYO6B//Icici Bank Limited -036001076406//netbanking
24,998.75
1,128.118
22.16
2,277.798
25-MAR-2025
Purchase Trxn.Ref.No.pay_QAvOAccUTyQsOD//Icici Bank Limited -036001076406//netbanking
24,998.75
1,126.928
22.18
3,404.726
04-APR-2025
Purchase Trxn.Ref.No.pay_QEtKkLZF7xIjT2//Icici Bank Limited -036001076406//netbanking
24,998.75
1,160.606
21.54
4,565.332
07-APR-2025
Purchase Trxn.Ref.No.pay_QG5dJuwGgIzamP//Icici Bank Limited -036001076406//netbanking
24,998.75
1,203.692
20.77
5,769.024
21-APR-2025
Purchase Trxn.Ref.No.pay_QLcozrm8yCA8OU//Icici Bank Limited -036001076406//netbanking
49,997.50
2,183.078
22.90
7,952.102
29-APR-2025
Purchase  pay_QORg6Sd8o3zvvN netbanking ICIC0000360 Icici BankLimited036001076406
24,998.75
1,083.459
23.07
9,035.561
06-MAY-2025
Purchase Trxn.Ref.No.pay_QRajF8MHCy8nql//Icici Bank Limited -036001076406//netbanking
49,997.50
2,207.327
22.65
11,242.888
14-MAY-2025
Purchase Trxn.Ref.No.pay_QUhseZACCoFAVW//Icici Bank Limited -036001076406//netbanking
24,998.75
1,050.756
23.79
12,293.644
16-MAY-2025
Purchase Trxn.Ref.No.pay_QVXMkzx5hjT8VC//Icici Bank Limited -036001076406//netbanking
24,998.75
1,034.708
24.16
13,328.352
20-MAY-2025
Purchase Trxn.Ref.No.pay_QX7mvhUhcl1xKV//Icici Bank Limited -036001076406//netbanking
24,998.75
1,049.644
23.82
14,377.996
22-MAY-2025
Sys. Investment ISIP (1/11)
24,998.75
1,044.841
23.93
15,422.837`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 18 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
02-JUN-2025
Sys. Investment ISIP (2/11)
24,998.75
1,020.707
24.49
16,443.544
09-JUN-2025
Sys. Investment ISIP (3/11)
24,998.75
989.822
25.26
17,433.366
16-JUN-2025
Sys. Investment ISIP (4/11)
24,998.75
1,003.849
24.90
18,437.215
23-JUN-2025
Sys. Investment ISIP (5/11)
24,998.75
1,014.243
24.65
19,451.458
01-JUL-2025
Sys. Investment ISIP (6/11)
24,998.75
987.554
25.31
20,439.012
08-JUL-2025
Sys. Investment ISIP (7/11)
24,998.75
990.218
25.25
21,429.230
15-JUL-2025
Sys. Investment ISIP (8/11)
24,998.75
986.257
25.35
22,415.487
22-JUL-2025
Sys. Investment ISIP (9/11)
24,998.75
990.210
25.25
23,405.697
25-JUL-2025
Purchase Trxn.Ref.No.pay_QxFIhbkpiYt3GQ//Icici Bank Limited -036001076406//netbanking
24,998.75
1,007.380
24.82
24,413.077
01-AUG-2025
Sys. Investment ISIP (10/11)
24,998.75
1,026.858
24.34
25,439.935
08-AUG-2025
Sys. Investment ISIP (11/11)
24,998.75
1,038.594
24.07
26,478.529
Closing Unit Balance: 26,478.529
Nav as on 07-AUG-2026: INR 27.0672
Valuation on 09-Aug-2026 : INR 7,16,699.64`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 19 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA NIFTY NEXT 50 JUNIOR BEES FOF - DIRECT GROWTH PLAN (Advisor: /DIRECT)  ISIN: INF204KB1X25
KYC : OK
Opening Unit Balance: 0.000
18-FEB-2025
Purchase Trxn.Ref.No.pay_Px55e404oUi5ps//Icici Bank Limited -036001076406//netbanking
99,995.00
4,486.394
22.29
4,486.394
21-FEB-2025
Purchase Trxn.Ref.No.pay_PyIhPtgH8b3BMN//Icici Bank Limited -036001076406//netbanking
49,997.50
2,207.483
22.65
6,693.877
27-FEB-2025
Purchase Trxn.Ref.No.pay_Q0eiFcfNgyh43d//Icici Bank Limited -036001076406//netbanking
49,997.50
2,270.962
22.02
8,964.839
10-MAR-2025
Purchase Trxn.Ref.No.pay_Q50LbOj3FTmZsl//Icici Bank Limited -036001076406//netbanking
24,998.75
1,131.513
22.09
10,096.352
19-MAR-2025
Purchase Trxn.Ref.No.pay_Q8Zfvf8p0T97O2//Icici Bank Limited -036001076406//netbanking
24,998.75
1,077.704
23.20
11,174.056
21-MAR-2025
Purchase Trxn.Ref.No.pay_Q9MsUPS3OyKAtS//Icici Bank Limited -036001076406//netbanking
49,997.50
2,127.119
23.50
13,301.175
25-MAR-2025
Purchase Trxn.Ref.No.pay_QAvKAvlyUUQF03//Icici Bank Limited -036001076406//netbanking
24,998.75
1,065.332
23.47
14,366.507
01-APR-2025
Purchase Trxn.Ref.No.pay_QDigqJBl9McnZ6//Icici Bank Limited -036001076406//netbanking
24,998.75
1,069.854
23.37
15,436.361
21-APR-2025
Purchase Trxn.Ref.No.pay_QLctXbnMbHAjeB//Icici Bank Limited -036001076406//netbanking
49,997.50
2,047.157
24.42
17,483.518
24-APR-2025
Purchase Trxn.Ref.No.pay_QMqRPbg3lz30tS-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
49,997.50
2,033.328
24.59
19,516.846
29-APR-2025
Purchase Trxn.Ref.No.pay_QOo0y6LO3T8N73-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
1,030.621
24.26
20,547.467
06-MAY-2025
Purchase Trxn.Ref.No.pay_QRag5qXnI8u5eX//Icici Bank Limited -036001076406//netbanking
49,997.50
2,092.410
23.89
22,639.877`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 20 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
09-MAY-2025
Purchase Trxn.Ref.No.pay_QSlhLtxWU5txiT//Icici Bank Limited -036001076406//netbanking
24,998.75
1,066.377
23.44
23,706.254
14-MAY-2025
Purchase Trxn.Ref.No.pay_QUhmJnBFwDxZ1S//Icici Bank Limited -036001076406//netbanking
24,998.75
1,018.209
24.55
24,724.463
16-MAY-2025
Purchase Trxn.Ref.No.pay_QVXIx16e8F6VMK//Icici Bank Limited -036001076406//netbanking
24,998.75
995.050
25.12
25,719.513
20-MAY-2025
Purchase Trxn.Ref.No.pay_QX7jg3kcRO9MOX//Icici Bank Limited -036001076406//netbanking
24,998.75
1,008.734
24.78
26,728.247
22-MAY-2025
Sys. Investment ISIP (1/14)
12,499.38
500.570
24.97
27,228.817
26-MAY-2025
Purchase Trxn.Ref.No.pay_QZUJqLM2viVZA6//Icici Bank Limited -036001076406//netbanking
24,998.75
991.550
25.21
28,220.367
02-JUN-2025
Sys. Investment ISIP (2/14)
12,499.38
498.067
25.10
28,718.434
03-JUN-2025
Purchase Trxn.Ref.No.pay_Qcf6ougbcCuaWd//Icici Bank Limited -036001076406//netbanking
24,998.75
1,001.095
24.97
29,719.529
09-JUN-2025
Purchase Trxn.Ref.No.pay_Qf0uWwAsSJlnYk//Icici Bank Limited -036001076406//netbanking
87,495.63
3,402.262
25.72
33,121.791
09-JUN-2025
Sys. Investment ISIP (3/14)
12,499.38
486.038
25.72
33,607.829
12-JUN-2025
Purchase Trxn.Ref.No.pay_QgECKa0NJRBKvq//Icici Bank Limited -036001076406//netbanking
24,998.75
988.011
25.30
34,595.840
16-JUN-2025
Sys. Investment ISIP (4/14)
12,499.38
494.496
25.28
35,090.336
17-JUN-2025
Purchase Trxn.Ref.No.pay_QiCa5XXdhBteVV//Icici Bank Limited -036001076406//netbanking
37,498.13
1,494.753
25.09
36,585.089
23-JUN-2025
Sys. Investment ISIP (5/14)
12,499.38
499.745
25.01
37,084.834`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 21 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
26-JUN-2025
Purchase Trxn.Ref.No.pay_QlkNeMEHsXzUgx//Icici Bank Limited -036001076406//netbanking
12,499.38
489.774
25.52
37,574.608
30-JUN-2025
Purchase Trxn.Ref.No.pay_QnLmGoZ5KFDH7e//Icici Bank Limited -036001076406//netbanking
24,998.75
970.027
25.77
38,544.635
01-JUL-2025
Sys. Investment ISIP (6/14)
12,499.38
485.324
25.75
39,029.959
03-JUL-2025
Purchase Trxn.Ref.No.pay_QoXoGrQJ2s2Gbd//Icici Bank Limited -036001076406//netbanking
24,998.75
975.409
25.63
40,005.368
08-JUL-2025
Sys. Investment ISIP (7/14)
12,499.38
485.435
25.75
40,490.803
15-JUL-2025
Sys. Investment ISIP (8/14)
12,499.38
485.605
25.74
40,976.408
17-JUL-2025
Purchase Trxn.Ref.No.pay_Qu5FNQZU8k3HdZ//Icici Bank Limited -036001076406//netbanking
12,499.38
485.409
25.75
41,461.817
22-JUL-2025
Sys. Investment ISIP (9/14)
12,499.38
488.457
25.59
41,950.274
25-JUL-2025
Purchase Trxn.Ref.No.pay_QxFCVtzWRMboUC//Icici Bank Limited -036001076406//netbanking
37,498.13
1,488.417
25.19
43,438.691
01-AUG-2025
Sys. Investment ISIP (10/14)
12,499.38
502.623
24.87
43,941.314
01-AUG-2025
Purchase Trxn.Ref.No.pay_R00ZhqeGbOg2km//Icici Bank Limited -036001076406//netbanking
12,499.38
502.623
24.87
44,443.937
06-AUG-2025
Purchase Trxn.Ref.No.pay_R1zirpqW9gR9Xz//Icici Bank Limited -036001076406//netbanking
24,998.75
1,005.424
24.86
45,449.361
08-AUG-2025
Sys. Investment ISIP (11/14)
12,499.38
507.674
24.62
45,957.035
18-AUG-2025
Sys. Investment ISIP (12/14)
12,499.38
493.495
25.33
46,450.530`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 22 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
22-AUG-2025
Purchase Trxn.Ref.No.pay_R8IuBOS73gLtNa//Icici Bank Limited -036001076406//netbanking
12,499.38
492.950
25.36
46,943.480
22-AUG-2025
Sys. Investment ISIP (13/14)
12,499.38
492.950
25.36
47,436.430
26-AUG-2025
Purchase
24,998.75
995.031
25.12
48,431.461
01-SEP-2025
Sys. Investment ISIP (14/14)
12,499.38
498.991
25.05
48,930.452
09-SEP-2025
Purchase Trxn.Ref.No.pay_RFRq9FlfAi73u8-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
12,499.38
493.169
25.35
49,423.621
02-FEB-2026
Purchase Trxn.Ref.No.pay_SBClrgbBHCd4ek//Icici Bank Limited -036001076406//netbanking
24,998.75
991.915
25.20
50,415.536
23-MAR-2026
Purchase Trxn.Ref.No.pay_SUb0QyXRoIePqF-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
1,080.373
23.14
51,495.909
Closing Unit Balance: 51,495.909
Nav as on 07-AUG-2026: INR 28.2009
Valuation on 09-Aug-2026 : INR 14,52,230.98`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 23 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA NIFTY SMALLCAP 250 INDEX FUND - DIRECT GROWTH PLAN (Advisor: /DIRECT)  ISIN: INF204KB15W0
KYC : OK
Opening Unit Balance: 0.000
14-FEB-2025
Purchase Trxn.Ref.No.pay_PvUjJunfMmi5hK//Icici Bank Limited -036001076406//netbanking
49,997.50
1,732.439
28.86
1,732.439
18-FEB-2025
Purchase Trxn.Ref.No.pay_Px5BLI83raatvh//Icici Bank Limited -036001076406//netbanking
49,997.50
1,760.612
28.40
3,493.051
28-FEB-2025
Purchase Trxn.Ref.No.pay_Q14VxTEqSOBhFO//Icici Bank Limited -036001076406//netbanking
24,998.75
907.139
27.56
4,400.190
10-MAR-2025
Purchase Trxn.Ref.No.pay_Q50QQjF7ouyH4R//Icici Bank Limited -036001076406//netbanking
24,998.75
876.946
28.51
5,277.136
19-MAR-2025
Purchase Trxn.Ref.No.pay_Q8ZmpAkKzg1Id1//Icici Bank Limited -036001076406//netbanking
24,998.75
848.497
29.46
6,125.633
25-MAR-2025
Sys. Investment ISIP (1/6)
24,998.75
827.422
30.21
6,953.055
04-APR-2025
Purchase Trxn.Ref.No.pay_QEtB7fFtcuyLrY//Icici Bank Limited -036001076406//netbanking
24,998.75
855.205
29.23
7,808.260
07-APR-2025
Purchase Trxn.Ref.No.pay_QG59z7cOF9EQgo//Icici Bank Limited -036001076406//netbanking
24,998.75
890.738
28.07
8,698.998
25-APR-2025
Sys. Investment ISIP (2/6)
24,998.75
814.597
30.69
9,513.595
06-MAY-2025
Purchase Trxn.Ref.No.pay_QRap5KAbcWe7H9//Icici Bank Limited -036001076406//netbanking
24,998.75
832.978
30.01
10,346.573
26-MAY-2025
Sys. Investment ISIP (3/6)
24,998.75
760.789
32.86
11,107.362
09-JUN-2025
Purchase Trxn.Ref.No.pay_Qf14YA9G3PfVXk//Icici Bank Limited -036001076406//netbanking
24,998.75
721.802
34.63
11,829.164`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 24 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
25-JUN-2025
Sys. Investment ISIP (4/6)
24,998.75
724.173
34.52
12,553.337
25-JUL-2025
Purchase Trxn.Ref.No.pay_QxFNSXygKIGOfr//Icici Bank Limited -036001076406//netbanking
24,998.75
726.959
34.39
13,280.296
25-JUL-2025
Sys. Investment ISIP (5/6)
24,998.75
726.959
34.39
14,007.255
28-JUL-2025
Purchase Trxn.Ref.No.pay_QyR5TnsDuAKALu//Icici Bank Limited -036001076406//netbanking
24,998.75
736.136
33.96
14,743.391
01-AUG-2025
Purchase Trxn.Ref.No.pay_R00ddtFpybcy4g//Icici Bank Limited -036001076406//netbanking
24,998.75
748.910
33.38
15,492.301
06-AUG-2025
Purchase Trxn.Ref.No.pay_R1zq8gCV29CFDM//Icici Bank Limited -036001076406//netbanking
24,998.75
752.977
33.20
16,245.278
25-AUG-2025
Sys. Investment ISIP (6/6)
24,998.75
739.617
33.80
16,984.895
26-AUG-2025
Purchase Trxn.Ref.No.pay_R9tuavMLStmY3g//Icici Bank Limited -036001076406//netbanking
24,998.75
753.578
33.17
17,738.473
26-SEP-2025
Purchase Trxn.Ref.No.pay_RMAh8OY2PkgcU3//Icici Bank Limited -036001076406//netbanking
24,998.75
756.131
33.06
18,494.604
08-DEC-2025
Purchase Trxn.Ref.No.pay_Rp49eMAFZH1WVq-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
49,997.50
1,565.744
31.93
20,060.348
27-JAN-2026
Purchase Trxn.Ref.No.pay_S8plSDAphlOjJS//Icici Bank Limited -036001076406//netbanking
24,998.75
822.495
30.39
20,882.843
23-MAR-2026
Purchase Trxn.Ref.No.pay_SUasEUKvWycO2w-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
886.556
28.20
21,769.399
Closing Unit Balance: 21,769.399
Nav as on 07-AUG-2026: INR 36.4069
Valuation on 09-Aug-2026 : INR 7,92,556.33`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 25 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA SMALL CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION (Advisor: /DIRECT)  ISIN: INF204K01K15
KYC : OK
Opening Unit Balance: 0.000`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 26 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
19-FEB-2025
Sys. Investment Trxn.Ref.No.pay_PxV43toCC3wVPC//ICICI BANK LIMITED -036001076406/netbanking (1/3)
9,999.50
61.340
163.02
61.340
24-MAR-2025
Sys. Investment ISIP (2/3)
9,999.50
58.215
171.77
119.555
25-MAR-2025
Sys. Investment Trxn.Ref.No.pay_QAsqi7ucItImif//ICICI BANK LIMITED -036001076406/netbanking (1/25)
24,998.75
147.478
169.51
267.033
01-APR-2025
Sys. Investment ISIP (3/3)
9,999.50
59.815
167.17
326.848
22-APR-2025
Sys. Investment ISIP (2/25)
24,998.75
141.549
176.61
468.397
02-MAY-2025
Sys. Investment ISIP (3/25)
24,998.75
144.904
172.52
613.301
08-MAY-2025
Sys. Investment ISIP (4/25)
24,998.75
146.313
170.86
759.614
15-MAY-2025
Sys. Investment ISIP (5/25)
24,998.75
138.045
181.09
897.659
22-MAY-2025
Sys. Investment ISIP (6/25)
24,998.75
136.856
182.66
1,034.515
02-JUN-2025
Sys. Investment ISIP (7/25)
24,998.75
134.032
186.51
1,168.547
09-JUN-2025
Sys. Investment ISIP (8/25)
24,998.75
131.118
190.66
1,299.665
16-JUN-2025
Sys. Investment ISIP (9/25)
24,998.75
132.498
188.67
1,432.163
23-JUN-2025
Sys. Investment ISIP (10/25)
24,998.75
133.376
187.43
1,565.539
01-JUL-2025
Sys. Investment ISIP (11/25)
24,998.75
128.824
194.05
1,694.363`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 27 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
08-JUL-2025
Sys. Investment ISIP (12/25)
24,998.75
129.192
193.50
1,823.555
15-JUL-2025
Sys. Investment ISIP (13/25)
24,998.75
127.834
195.56
1,951.389
22-JUL-2025
Sys. Investment ISIP (14/25)
24,998.75
128.148
195.08
2,079.537
01-AUG-2025
Sys. Investment ISIP (15/25)
24,998.75
133.202
187.68
2,212.739
08-AUG-2025
Sys. Investment ISIP (16/25)
24,998.75
135.424
184.60
2,348.163
18-AUG-2025
Sys. Investment ISIP (17/25)
24,998.75
133.261
187.59
2,481.424
22-AUG-2025
Sys. Investment ISIP (18/25)
24,998.75
132.090
189.26
2,613.514
01-SEP-2025
Sys. Investment ISIP (19/25)
24,998.75
133.557
187.18
2,747.071
08-SEP-2025
Sys. Investment ISIP (20/25)
24,998.75
132.664
188.44
2,879.735
15-SEP-2025
Sys. Investment ISIP (21/25)
24,998.75
130.578
191.45
3,010.313
22-SEP-2025
Sys. Investment ISIP (22/25)
24,998.75
129.929
192.40
3,140.242
01-OCT-2025
Sys. Investment ISIP (23/25)
24,998.75
133.335
187.49
3,273.577
08-OCT-2025
Sys. Investment ISIP (24/25)
24,998.75
132.721
188.36
3,406.298
15-OCT-2025
Sys. Investment ISIP (25/25)
24,998.75
132.389
188.83
3,538.687`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 28 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
16-APR-2026
Redemption Directly credited to your Bank account (ICICI BANK-DCB)
(4,918.00)
(26.834)
186.34
3,511.853
Closing Unit Balance: 3,511.853
Nav as on 07-AUG-2026: INR 208.2889
Valuation on 09-Aug-2026 : INR 7,31,480.00`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 29 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
No Folios Found`,
	`Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 30 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
---
--- No Folios Found ---
---
---
---
---
#IDCW - Income Distribution cum Capital Withdrawal
*SoA - Statement of Account`,
}

func TestParseMFCentral_FullCleanRealDocument(t *testing.T) {
	result := ParseMFCentral(fullCleanDocPages)

	t.Logf("staged: %d, manual review: %d", len(result.Staged), len(result.ManualReview))
	for _, m := range result.ManualReview {
		t.Logf("REVIEW [folio %s] %s :: %s", m.Folio, m.Reason, m.Text)
	}

	if len(result.ManualReview) != 0 {
		t.Errorf("expected 0 manual review lines on the real document, got %d", len(result.ManualReview))
	}

	wantISINs := []string{
		"INF204K01E54", "INF204K01H36", "INF204K01XF9", "INF204KB12Z0",
		"INF204KC1DG5", "INF204KB18Z7", "INF204KB1X25", "INF204KB15W0", "INF204K01K15",
	}
	seen := map[string]int{}
	for _, s := range result.Staged {
		seen[s.Txn.ISIN]++
	}
	for _, want := range wantISINs {
		if seen[want] == 0 {
			t.Errorf("expected at least one transaction for ISIN %s, found none", want)
		}
	}
	for isin, n := range seen {
		fmt.Printf("ISIN %s: %d transactions\n", isin, n)
	}

	var redemptions int
	for _, s := range result.Staged {
		if s.Txn.Type == "REDEMPTION" {
			redemptions++
		}
	}
	if redemptions != 1 {
		t.Errorf("expected exactly 1 redemption, got %d", redemptions)
	}
}
