FROM ubuntu
ARG package
COPY keys/pubkey.gpg /usr/share/debsig/keyrings/BC8ACDD415BD80B3/debsig.gpg
COPY ${package} /tmp/foo.deb

RUN apt update -y
RUN apt install -y debsig-verify
RUN mkdir -p /etc/debsig/policies/BC8ACDD415BD80B3
RUN echo '<?xml version="1.0"?>\n\
<!DOCTYPE Policy SYSTEM "https://www.debian.org/debsig/1.0/policy.dtd">\n\
<Policy xmlns="https://www.debian.org/debsig/1.0/">\n\
\n\
  <Origin Name="test" id="BC8ACDD415BD80B3" Description="Test package"/>\n\
\n\
  <Selection>\n\
    <Required Type="origin" File="debsig.gpg" id="BC8ACDD415BD80B3"/>\n\
  </Selection>\n\
\n\
   <Verification MinOptional="0">\n\
    <Required Type="origin" File="debsig.gpg" id="BC8ACDD415BD80B3"/>\n\
   </Verification>\n\
</Policy>\n\
\n' >> /etc/debsig/policies/BC8ACDD415BD80B3/policy.pol

# manually check signature
RUN debsig-verify /tmp/foo.deb | grep "debsig: Verified package from 'Test package' (test)"

# clear dpkg config as it contains 'no-debsig', now every
# package that will be installed must be signed
RUN echo "" > /etc/dpkg/dpkg.cfg
RUN dpkg -i /tmp/foo.deb
