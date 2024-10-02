make
rm hs_log* 
./hotstuff run &>> hs_log.txt
cat hs_log.txt | grep "hs1" > hs_log1.txt
