DT=$(date '+%Y-%m-%d_%H-%M-%S')
DIR="debug_logs"
FILE="$DIR/$DT.txt"
make
mkdir $DIR
echo "Writing to $FILE"
./hotstuff --log-level=debug run &>> $FILE
