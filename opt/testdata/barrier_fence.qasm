// Barriers are optimization fences: nothing here may be touched, even
// though every pair would cancel or merge without the barriers.
OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
h q[0];
barrier q;
h q[0];
rz(pi/2) q[1];
barrier q[0];
rz(-pi/2) q[1];
