<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiAdminApplicationsByCodeByCode extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $code;
    /**
     * @param string $code
     */
    public function __construct(string $code)
    {
        $this->code = $code;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{code}'], [$this->code], '/api/admin/applications/by-code/{code}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminApplicationsByCodeByCodeNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse200', 'json');
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiAdminApplicationsByCodeByCodeNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}