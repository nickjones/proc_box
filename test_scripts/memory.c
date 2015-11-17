#include <stdlib.h>

int main() {
    int i = 0;
    char *p = calloc((1024*1024*1024), sizeof(char));
    // Have to touch it before it actually loads resident memory.
    //p[0] = 0xFF;
    while(1){} // Wait to be killed. 
}
