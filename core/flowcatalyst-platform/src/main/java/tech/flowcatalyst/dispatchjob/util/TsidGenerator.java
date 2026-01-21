package tech.flowcatalyst.dispatchjob.util;

import com.github.f4b6a3.tsid.TsidCreator;

public class TsidGenerator {

    public static String generate() {
        return TsidCreator.getTsid().toString();
    }
}
