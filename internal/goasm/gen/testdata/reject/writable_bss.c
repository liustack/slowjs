// A mutable global: the engine keeps all state in JSRuntime.
long counter;
long f(void *sp) { (void)sp; return ++counter; }
