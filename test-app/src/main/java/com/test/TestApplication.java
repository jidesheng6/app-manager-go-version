package com.test;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.time.LocalDateTime;
import java.time.format.DateTimeFormatter;

@SpringBootApplication
@RestController
public class TestApplication {

    public static void main(String[] args) {
        System.out.println("=== 测试应用启动中 ===");
        System.out.println("启动时间: " + LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")));
        SpringApplication.run(TestApplication.class, args);
        System.out.println("=== 测试应用启动完成 ===");
    }

    @GetMapping("/")
    public String home() {
        return "测试应用运行正常！当前时间: " + LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss"));
    }

    @GetMapping("/health")
    public String health() {
        return "OK";
    }

    @GetMapping("/info")
    public String info() {
        return "这是一个用于测试app-manager端口检测功能的Spring Boot应用";
    }
}