FROM ubuntu

RUN apt-get update
RUN apt-get install -y openssh-server

ADD entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh", "sleep", "infinity" ]
