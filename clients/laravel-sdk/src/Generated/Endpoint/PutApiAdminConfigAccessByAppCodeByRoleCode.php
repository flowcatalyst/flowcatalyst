<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiAdminConfigAccessByAppCodeByRoleCode extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $roleCode;
    /**
     * @param string $appCode
     * @param string $roleCode
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutBody $requestBody
     */
    public function __construct(string $appCode, string $roleCode, ?\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutBody $requestBody = null)
    {
        $this->appCode = $appCode;
        $this->roleCode = $roleCode;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{roleCode}'], [$this->appCode, $this->roleCode], '/api/admin/config-access/{appCode}/{roleCode}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutBody) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200', 'json');
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}