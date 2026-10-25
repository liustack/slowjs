// Thread-local storage.
_Thread_local long t;
long f(void *sp) { (void)sp; return ++t; }
