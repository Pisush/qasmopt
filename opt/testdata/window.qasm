// Commutation-aware window: pairs separated by ops on disjoint qubits
// cancel; a blocking op on a shared qubit prevents it.
OPENQASM 2.0;
include "qelib1.inc";
qreg q[3];
h q[0];
x q[1];
t q[2];
h q[0];
cx q[0], q[1];
z q[2];
cx q[0], q[1];
h q[1];
s q[1];
h q[1];
rz(pi/8) q[2];
x q[0];
rz(pi/8) q[2];
