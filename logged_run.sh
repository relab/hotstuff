DT=$(date '+%Y-%m-%d_%H-%M-%S')
FILE="alans_logs/log_$DT.txt"
echo $FILE
make
mkdir alans_logs
./hotstuff --log-level=debug run &>> $FILE
