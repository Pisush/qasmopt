// Rotation merging: sums, exact cancellation, 2*pi drop, u1 fusion.
OPENQASM 2.0;
include "qelib1.inc";
qreg q[3];
rz(pi/4) q[0];
rz(pi/4) q[0];
rx(pi/3) q[1];
rx(-pi/3) q[1];
ry(pi) q[2];
ry(pi) q[2];
u1(0.25) q[0];
u1(0.5) q[0];
rz(0.5) q[1];
rx(0.5) q[1];
