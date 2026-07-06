// Adjacent inverse pairs of every supported kind, plus survivors.
OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
h q[0];
h q[0];
x q[1];
x q[1];
s q[0];
sdg q[0];
t q[1];
tdg q[1];
cx q[0], q[1];
cx q[0], q[1];
cx q[0], q[1];
cx q[1], q[0];
h q[0];
