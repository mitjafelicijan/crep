#ifndef TEST_CUH
#define TEST_CUH

struct Point {
    float x;
    float y;
    float z;
};

class Dim3 {
public:
    int x;
    int y;
    int z;
};

__device__ void device_func() {}

#endif
