// A read-only table other than a constant pool (a switch jump table):
// not supported by the prototype yet, so it must fail loudly.
long f(void *sp, long x) {
  (void)sp;
  switch (x) {
  case 0: return 11; case 1: return 23; case 2: return 37; case 3: return 41;
  case 4: return 53; case 5: return 67; case 6: return 71; case 7: return 89;
  }
  return 0;
}
